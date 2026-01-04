package serpent

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/ebitengine/purego"
)

// Types used in the Python C API.
type pyObject uintptr
type pyThreadState uintptr

// pyInterpreterConfig is the configuration for creating a sub-interpreter.
// This matches the PyInterpreterConfig struct in Python 3.12+.
type pyInterpreterConfig struct {
	useMainObmalloc     int32
	allowFork           int32
	allowExec           int32
	allowThreads        int32
	allowDaemonThreads  int32
	checkMultiInterpExt int32
	gil                 int32
}

// pyStatus represents the result of a Python C API call.
type pyStatus struct {
	typ      int32
	func_    uintptr
	err_msg  uintptr
	exitcode int32
}

// Constants used in the Python C API.
const (
	pyFileInput               = 257
	pyInterpreterConfigOwnGIL = 2
)

// Function prototypes for the Python C API.
var py_InitializeEx func(int)
var py_Finalize func()
var pyEval_GetBuiltins func() pyObject
var pyRun_String func(string, int, pyObject, pyObject) pyObject
var pyErr_Occurred func() bool
var pyErr_Print func()
var pyErr_Fetch func(*pyObject, *pyObject, *pyObject)
var pyErr_Clear func()
var pyObject_Str func(pyObject) pyObject
var pyObject_Call func(pyObject, pyObject, pyObject) pyObject
var pyObject_GetAttrString func(pyObject, string) pyObject
var pyDict_New func() pyObject
var pyDict_GetItemString func(pyObject, string) pyObject
var pyDict_SetItemString func(pyObject, string, pyObject) int
var pyUnicode_AsUTF8 func(pyObject) string
var pyUnicode_FromString func(string) pyObject
var pyTuple_New func(int) pyObject
var pyTuple_SetItem func(pyObject, int, pyObject) int
var pyImport_ImportModule func(string) pyObject
var py_DecRef func(pyObject)
var py_IncRef func(pyObject)
var py_GetVersion func() string
var py_NewInterpreterFromConfig func(*pyThreadState, *pyInterpreterConfig) pyStatus
var py_EndInterpreter func(pyThreadState)
var pyThreadState_Swap func(pyThreadState) pyThreadState
var pyThreadState_Get func() pyThreadState
var pyEval_SaveThread func() pyThreadState
var pyEval_RestoreThread func(pyThreadState)

// python is a handle to the Python shared library.
var python uintptr

// workerPool is the global pool of Python workers.
var workerPool *pool

// worker represents a Python sub-interpreter running on a dedicated OS thread.
type worker struct {
	id       int
	interp   pyThreadState
	requests chan *execContext
	initErr  error
	ready    chan struct{}
	done     chan struct{}
}

// pool manages a collection of workers.
type pool struct {
	workers []*worker
	next    atomic.Uint64
	closed  atomic.Bool
}

// initPython initializes the Python library and registers C API functions.
// Returns whether sub-interpreters are supported.
func initPython(libraryPath string) (bool, error) {
	if python != 0 {
		return false, ErrAlreadyInitialized
	}

	lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false, fmt.Errorf("dlopen: %v", err)
	}
	python = lib

	// Register core Python C API functions
	purego.RegisterLibFunc(&py_InitializeEx, python, "Py_InitializeEx")
	purego.RegisterLibFunc(&py_Finalize, python, "Py_Finalize")
	purego.RegisterLibFunc(&pyEval_GetBuiltins, python, "PyEval_GetBuiltins")
	purego.RegisterLibFunc(&pyErr_Occurred, python, "PyErr_Occurred")
	purego.RegisterLibFunc(&pyErr_Print, python, "PyErr_Print")
	purego.RegisterLibFunc(&pyErr_Fetch, python, "PyErr_Fetch")
	purego.RegisterLibFunc(&pyErr_Clear, python, "PyErr_Clear")
	purego.RegisterLibFunc(&pyObject_Str, python, "PyObject_Str")
	purego.RegisterLibFunc(&pyObject_Call, python, "PyObject_Call")
	purego.RegisterLibFunc(&pyObject_GetAttrString, python, "PyObject_GetAttrString")
	purego.RegisterLibFunc(&pyDict_New, python, "PyDict_New")
	purego.RegisterLibFunc(&pyDict_GetItemString, python, "PyDict_GetItemString")
	purego.RegisterLibFunc(&pyDict_SetItemString, python, "PyDict_SetItemString")
	purego.RegisterLibFunc(&pyUnicode_AsUTF8, python, "PyUnicode_AsUTF8")
	purego.RegisterLibFunc(&pyUnicode_FromString, python, "PyUnicode_FromString")
	purego.RegisterLibFunc(&pyTuple_New, python, "PyTuple_New")
	purego.RegisterLibFunc(&pyTuple_SetItem, python, "PyTuple_SetItem")
	purego.RegisterLibFunc(&pyImport_ImportModule, python, "PyImport_ImportModule")
	purego.RegisterLibFunc(&py_DecRef, python, "Py_DecRef")
	purego.RegisterLibFunc(&py_IncRef, python, "Py_IncRef")
	purego.RegisterLibFunc(&pyRun_String, python, "PyRun_String")
	purego.RegisterLibFunc(&py_GetVersion, python, "Py_GetVersion")

	supportsSubInterpreters := platformSupportsSubInterpreters && checkPythonVersion()
	if supportsSubInterpreters {
		purego.RegisterLibFunc(&py_NewInterpreterFromConfig, python, "Py_NewInterpreterFromConfig")
		purego.RegisterLibFunc(&py_EndInterpreter, python, "Py_EndInterpreter")
		purego.RegisterLibFunc(&pyThreadState_Swap, python, "PyThreadState_Swap")
		purego.RegisterLibFunc(&pyThreadState_Get, python, "PyThreadState_Get")
		purego.RegisterLibFunc(&pyEval_SaveThread, python, "PyEval_SaveThread")
		purego.RegisterLibFunc(&pyEval_RestoreThread, python, "PyEval_RestoreThread")
	}

	return supportsSubInterpreters, nil
}

// checkInit checks if the Python interpreter has been initialized. It panics if it has not.
func checkInit() {
	if python == 0 {
		panic(PythonNotInitialized("serpent: Init must be called before Run"))
	}
}

// checkPythonVersion checks if Python >= 3.12 for sub-interpreter support.
func checkPythonVersion() bool {
	py_InitializeEx(0)
	version := py_GetVersion()
	py_Finalize()

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}

	minorStr := parts[1]
	for i, c := range minorStr {
		if c < '0' || c > '9' {
			minorStr = minorStr[:i]
			break
		}
	}
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return false
	}

	return major > 3 || (major == 3 && minor >= 12)
}

// initSingleWorker initializes a single worker for interpreters that do not support sub-interpreters.
func initSingleWorker() error {
	w := &worker{
		id:       0,
		requests: make(chan *execContext, 100),
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}
	workerPool.workers = append(workerPool.workers, w)

	go startSingleWorker(w)
	<-w.ready
	return w.initErr
}

// initWithSubInterpreters initializes multiple workers with sub-interpreters.
func initWithSubInterpreters(numWorkers int) error {
	mainReady := make(chan struct{})
	var mainState pyThreadState

	go func() {
		runtime.LockOSThread()
		py_InitializeEx(0)
		mainState = pyThreadState_Get()
		pyEval_SaveThread()
		close(mainReady)

		for !workerPool.closed.Load() {
			runtime.Gosched()
		}

		pyEval_RestoreThread(mainState)
		py_Finalize()
	}()

	<-mainReady

	var initErrors []error
	for i := 0; i < numWorkers; i++ {
		w := &worker{
			id:       i,
			requests: make(chan *execContext, 100),
			ready:    make(chan struct{}),
			done:     make(chan struct{}),
		}

		go startSubInterpreterWorker(w)
		<-w.ready

		if w.initErr != nil {
			initErrors = append(initErrors, fmt.Errorf("worker %d: %w", i, w.initErr))
		} else {
			workerPool.workers = append(workerPool.workers, w)
		}
	}

	if len(workerPool.workers) == 0 {
		return fmt.Errorf("all workers failed to initialize: %w", errors.Join(initErrors...))
	}

	if len(initErrors) > 0 {
		return fmt.Errorf("some workers failed to initialize (continuing with %d workers): %w",
			len(workerPool.workers), errors.Join(initErrors...))
	}

	return nil
}

// startSingleWorker runs a single worker using the single-interpreter approach.
func startSingleWorker(w *worker) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	py_InitializeEx(0)
	defer py_Finalize()

	close(w.ready)
	for req := range w.requests {
		req.execute()
	}

	close(w.done)
}

// startSubInterpreterWorker runs a worker with its own sub-interpreter.
func startSubInterpreterWorker(w *worker) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	config := pyInterpreterConfig{
		useMainObmalloc:     0,
		allowFork:           0,
		allowExec:           0,
		allowThreads:        1,
		allowDaemonThreads:  0,
		checkMultiInterpExt: 1,
		gil:                 pyInterpreterConfigOwnGIL,
	}

	var tstate pyThreadState
	status := py_NewInterpreterFromConfig(&tstate, &config)
	if status.typ != 0 {
		w.initErr = ErrSubInterpreterFailed
		close(w.ready)
		close(w.done)
		return
	}

	w.interp = tstate

	close(w.ready)
	for req := range w.requests {
		req.execute()
	}

	py_EndInterpreter(w.interp)
	close(w.done)
}

// fetchPythonError retrieves the current Python exception and returns it as a Go error.
// It clears the Python error state after fetching.
func fetchPythonError() error {
	var ptype, pvalue, ptraceback pyObject
	pyErr_Fetch(&ptype, &pvalue, &ptraceback)
	if pvalue == 0 {
		pyErr_Clear()
		return ErrRunFailed
	}

	strObj := pyObject_Str(pvalue)
	var msg string
	if strObj != 0 {
		msg = pyUnicode_AsUTF8(strObj)
		py_DecRef(strObj)
	}

	if ptype != 0 {
		py_DecRef(ptype)
	}
	if pvalue != 0 {
		py_DecRef(pvalue)
	}
	if ptraceback != 0 {
		py_DecRef(ptraceback)
	}

	if msg == "" {
		return ErrRunFailed
	}
	return fmt.Errorf("%w: %s", ErrRunFailed, msg)
}

// callRun invokes the run function defined in globals with the JSON input,
// and returns the JSON-serialized result.
func callRun(globals pyObject, jsonInput string) (string, error) {
	runfn := pyDict_GetItemString(globals, "run")
	if runfn == 0 {
		return "", fmt.Errorf("%w: run() function not defined", ErrRunFailed)
	}

	json := pyImport_ImportModule("json")
	if json == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to import json module", ErrRunFailed)
	}
	defer py_DecRef(json)

	loadsfn := pyObject_GetAttrString(json, "loads")
	if loadsfn == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to get json.loads", ErrRunFailed)
	}
	defer py_DecRef(loadsfn)

	dumpsfn := pyObject_GetAttrString(json, "dumps")
	if dumpsfn == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to get json.dumps", ErrRunFailed)
	}
	defer py_DecRef(dumpsfn)

	input := pyUnicode_FromString(jsonInput)
	if input == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to create input string", ErrRunFailed)
	}
	defer py_DecRef(input)

	loadsArgs := pyTuple_New(1)
	if loadsArgs == 0 {
		return "", fmt.Errorf("%w: failed to create loads args tuple", ErrRunFailed)
	}
	py_IncRef(input)
	pyTuple_SetItem(loadsArgs, 0, input)

	parsedInput := pyObject_Call(loadsfn, loadsArgs, 0)
	py_DecRef(loadsArgs)
	if parsedInput == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to parse input JSON", ErrRunFailed)
	}
	defer py_DecRef(parsedInput)

	runArgs := pyTuple_New(1)
	if runArgs == 0 {
		return "", fmt.Errorf("%w: failed to create run args tuple", ErrRunFailed)
	}
	py_IncRef(parsedInput)
	pyTuple_SetItem(runArgs, 0, parsedInput)

	result := pyObject_Call(runfn, runArgs, 0)
	py_DecRef(runArgs)
	if result == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: run() returned NULL", ErrRunFailed)
	}
	defer py_DecRef(result)

	dumpsArgs := pyTuple_New(1)
	if dumpsArgs == 0 {
		return "", fmt.Errorf("%w: failed to create dumps args tuple", ErrRunFailed)
	}
	py_IncRef(result)
	pyTuple_SetItem(dumpsArgs, 0, result)

	jsonResult := pyObject_Call(dumpsfn, dumpsArgs, 0)
	py_DecRef(dumpsArgs)
	if jsonResult == 0 {
		if pyErr_Occurred() {
			return "", fetchPythonError()
		}
		return "", fmt.Errorf("%w: failed to serialize result to JSON", ErrRunFailed)
	}
	defer py_DecRef(jsonResult)

	resultStr := pyUnicode_AsUTF8(jsonResult)
	return resultStr, nil
}
