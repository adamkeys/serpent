// Package serpent provides functions for interacting with a Python interpreter.
package serpent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
var pyDict_New func() pyObject
var pyDict_GetItemString func(pyObject, string) pyObject
var pyDict_SetItemString func(pyObject, string, pyObject) int
var pyUnicode_AsUTF8 func(pyObject) string
var py_DecRef func(pyObject)
var py_GetVersion func() string
var py_NewInterpreterFromConfig func(*pyThreadState, *pyInterpreterConfig) pyStatus
var py_EndInterpreter func(pyThreadState)
var pyThreadState_Swap func(pyThreadState) pyThreadState
var pyThreadState_Get func() pyThreadState
var pyEval_SaveThread func() pyThreadState
var pyEval_RestoreThread func(pyThreadState)

var (
	// ErrAlreadyInitialized is returned when the Python interpreter is initialized more than once.
	ErrAlreadyInitialized = errors.New("already initialized")
	// ErrRunFailed is returned when the Python program fails to run.
	ErrRunFailed = errors.New("run failed")
	// ErrNoResult is returned when the result variable is not found in the Python program.
	ErrNoResult = errors.New("no result")
	// ErrSubInterpreterFailed is returned when a sub-interpreter fails to initialize.
	ErrSubInterpreterFailed = errors.New("sub-interpreter creation failed")
	// ErrNoHealthyWorkers is returned when all workers have failed.
	ErrNoHealthyWorkers = errors.New("no healthy workers available")
	// ErrNotInitialized is returned when Close is called before Init.
	ErrNotInitialized = errors.New("not initialized")
)

// python is a handle to the Python shared library.
var python uintptr

// supportsSubInterpreters indicates whether Python 3.12+ sub-interpreters with per-interpreter GIL are available.
var supportsSubInterpreters bool

// workerPool is the global pool of Python workers.
var workerPool *pool

// worker represents a Python sub-interpreter running on a dedicated OS thread.
type worker struct {
	id       int
	interp   pyThreadState
	requests chan *runContext
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

// Init initializes the Python interpreter with runtime.NumCPU() workers.
// This must be called before any other functions in this package.
func Init(libraryPath string) error {
	if python != 0 {
		return ErrAlreadyInitialized
	}

	lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("dlopen: %v", err)
	}
	python = lib

	// Register core Python C API functions
	purego.RegisterLibFunc(&py_InitializeEx, python, "Py_InitializeEx")
	purego.RegisterLibFunc(&py_Finalize, python, "Py_Finalize")
	purego.RegisterLibFunc(&pyEval_GetBuiltins, python, "PyEval_GetBuiltins")
	purego.RegisterLibFunc(&pyErr_Occurred, python, "PyErr_Occurred")
	purego.RegisterLibFunc(&pyErr_Print, python, "PyErr_Print")
	purego.RegisterLibFunc(&pyDict_New, python, "PyDict_New")
	purego.RegisterLibFunc(&pyDict_GetItemString, python, "PyDict_GetItemString")
	purego.RegisterLibFunc(&pyDict_SetItemString, python, "PyDict_SetItemString")
	purego.RegisterLibFunc(&pyUnicode_AsUTF8, python, "PyUnicode_AsUTF8")
	purego.RegisterLibFunc(&py_DecRef, python, "Py_DecRef")
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

	numWorkers := runtime.NumCPU()
	workerPool = &pool{
		workers: make([]*worker, 0, numWorkers),
	}
	if supportsSubInterpreters && numWorkers > 1 {
		return initWithSubInterpreters(numWorkers)
	}
	return initSingleWorker()
}

// Run runs a [Program] with the supplied argument and returns the result. The Python code must assign
// a result variable in the main program.
//
// Example Python program:
//
//	result = input + 1
func Run[TInput, TResult any](program Program[TInput, TResult], arg TInput) (TResult, error) {
	checkInit()

	input, err := json.Marshal(arg)
	if err != nil {
		return *new(TResult), fmt.Errorf("marshal input: %w", err)
	}
	code := generateCode(string(program), input)

	result, err := run(code)
	if err != nil {
		return *new(TResult), err
	}

	var value TResult
	if err := json.Unmarshal([]byte(result), &value); err != nil {
		return *new(TResult), fmt.Errorf("unmarshal result: %w", err)
	}

	return value, nil
}

// RunWrite runs a [Program] with the supplied argument with the Python program writing to the supplied writer.
// The writer is made available as a file descriptor (fd) in the Python program. The Python program must close
// the file descriptor when it is done writing.
//
// Example Python program:
//
//	import os
//	os.write(fd, b'OK')
//	os.close(fd)
func RunWrite[TInput any](w io.Writer, program Program[TInput, Writer], arg TInput) error {
	checkInit()

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pr.Close()
		io.Copy(w, pr)
	}()

	input, err := json.Marshal(struct {
		Input TInput
		Fd    uintptr
	}{arg, pw.Fd()})
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}
	code := generateWriterCode(string(program), input)

	_, err = run(code)
	if !errors.Is(err, ErrNoResult) {
		return err
	}

	if err := pw.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}
	wg.Wait()

	return nil
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
		requests: make(chan *runContext, 100),
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
	}
	workerPool.workers = append(workerPool.workers, w)

	go startLegacyWorker(w)
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
			requests: make(chan *runContext, 100),
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

// startLegacyWorker runs a single worker using the legacy single-interpreter approach.
func startLegacyWorker(w *worker) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	py_InitializeEx(0)
	defer py_Finalize()

	close(w.ready)
	for ctx := range w.requests {
		executeCode(ctx)
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
	for ctx := range w.requests {
		executeCode(ctx)
	}

	py_EndInterpreter(w.interp)
	close(w.done)
}

// executeCode runs Python code and populates the runContext with results.
func executeCode(ctx *runContext) {
	ctx.cond.L.Lock()

	globals := pyDict_New()
	pyDict_SetItemString(globals, "__builtins__", pyEval_GetBuiltins())

	pyRun_String(ctx.code, pyFileInput, globals, globals)
	if pyErr_Occurred() {
		pyErr_Print()
		ctx.err = ErrRunFailed
	} else if item := pyDict_GetItemString(globals, "_result"); item != 0 {
		ctx.value = pyUnicode_AsUTF8(item)
	} else {
		ctx.err = ErrNoResult
	}

	ctx.done = true
	ctx.cond.Signal()
	ctx.cond.L.Unlock()

	py_DecRef(globals)
}

// Close shuts down the Python interpreter and all workers.
func Close() error {
	if python == 0 {
		return ErrNotInitialized
	}

	if workerPool == nil {
		return ErrNotInitialized
	}

	workerPool.closed.Store(true)
	for _, w := range workerPool.workers {
		close(w.requests)
	}
	for _, w := range workerPool.workers {
		<-w.done
	}

	python = 0
	workerPool = nil

	return nil
}

// runContext identifies the context of a Python run.
type runContext struct {
	code string

	cond *sync.Cond
	done bool

	value string
	err   error
}

// run runs a Python program and returns the result.
func run(code string) (string, error) {
	if workerPool == nil || len(workerPool.workers) == 0 {
		return "", ErrNoHealthyWorkers
	}

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	cond.L.Lock()
	defer cond.L.Unlock()

	ctx := &runContext{code: code, cond: cond}

	idx := workerPool.next.Add(1) % uint64(len(workerPool.workers))
	w := workerPool.workers[idx]
	if w.initErr != nil {
		return "", fmt.Errorf("%w: %v", ErrSubInterpreterFailed, w.initErr)
	}

	w.requests <- ctx
	for !ctx.done {
		cond.Wait()
	}

	return ctx.value, ctx.err
}

// PythonNotInitialized is a panic type indicating that the Python interpreter has not been initialized.
type PythonNotInitialized string

// checkInit checks if the Python interpreter has been initialized. It panics if it has not.
func checkInit() {
	if python == 0 {
		panic(PythonNotInitialized("serpent: Init must be called before Run"))
	}
}
