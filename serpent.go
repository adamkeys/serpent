// Package serpent provides functions for interacting with a Python interpreter.
package serpent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

// Types used in the Python C API.
type pyObject uintptr
type pyThreadState uintptr

// Function prototypes for the Python C API.
var py_InitializeEx func(int)
var py_NewInterpreter func() pyThreadState
var py_EndInterpreter func(pyThreadState)
var pyThreadState_Swap func(pyThreadState)
var pyEval_InitThreads func()
var pyErr_Occurred func() bool
var pyErr_Print func()
var pyDict_New func() pyObject
var pyDict_Copy func(pyObject) pyObject
var pyDict_GetItemString func(pyObject, string) pyObject
var pyDict_SetItemString func(pyObject, string, pyObject) int
var pyUnicode_FromString func(string) pyObject
var pyUnicode_AsUTF8 func(pyObject) string
var pyLong_FromLong func(int) pyObject
var pyLong_AsLong func(pyObject) int
var py_DecRef func(pyObject)
var pyRun_String func(string, int, pyObject, pyObject) pyObject

var (
	// ErrRunFailed is returned when the Python program fails to run.
	ErrRunFailed = errors.New("run failed")
	// ErrNoResult is returned when the result variable is not found in the Python program.
	ErrNoResult = errors.New("no result")
)

// Constants used in the Python C API.
const pyFileInput = 257

// python is a handle to the Python shared library.
var python uintptr

// Init initializes the Python interpreter, loading the Python shared library from the supplied path. This
// must be called before any other functions in this package.
func Init(libraryPath string) error {
	lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("dlopen: %v", err)
	}
	python = lib

	purego.RegisterLibFunc(&py_InitializeEx, python, "Py_InitializeEx")
	purego.RegisterLibFunc(&pyEval_InitThreads, python, "PyEval_InitThreads")
	purego.RegisterLibFunc(&py_NewInterpreter, python, "Py_NewInterpreter")
	purego.RegisterLibFunc(&py_EndInterpreter, python, "Py_EndInterpreter")
	purego.RegisterLibFunc(&pyThreadState_Swap, python, "PyThreadState_Swap")
	purego.RegisterLibFunc(&pyErr_Occurred, python, "PyErr_Occurred")
	purego.RegisterLibFunc(&pyErr_Print, python, "PyErr_Print")
	purego.RegisterLibFunc(&pyDict_New, python, "PyDict_New")
	purego.RegisterLibFunc(&pyDict_Copy, python, "PyDict_Copy")
	purego.RegisterLibFunc(&pyDict_GetItemString, python, "PyDict_GetItemString")
	purego.RegisterLibFunc(&pyDict_SetItemString, python, "PyDict_SetItemString")
	purego.RegisterLibFunc(&pyUnicode_FromString, python, "PyUnicode_FromString")
	purego.RegisterLibFunc(&pyUnicode_AsUTF8, python, "PyUnicode_AsUTF8")
	purego.RegisterLibFunc(&pyLong_FromLong, python, "PyLong_FromLong")
	purego.RegisterLibFunc(&pyLong_AsLong, python, "PyLong_AsLong")
	purego.RegisterLibFunc(&py_DecRef, python, "Py_DecRef")
	purego.RegisterLibFunc(&pyRun_String, python, "PyRun_String")

	py_InitializeEx(0)

	return nil
}

// Run runs a [Program] with the supplied argument and returns the result. The Python code must assign
// a result variable in the main program.
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

// RunFd runs a [Program] with the supplied argument with the Python program writing to the supplied writer.
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
		return fmt.Errorf("run: %w (expected: %v)", err, ErrNoResult)
	}

	if err := pw.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}

	wg.Wait()
	return nil
}

// run runs the Python program and returns the result.
func run(code string) (value string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := py_NewInterpreter()
	pyThreadState_Swap(state)
	defer py_EndInterpreter(state)

	global := pyDict_New()
	defer py_DecRef(global)
	local := pyDict_New()
	defer py_DecRef(local)

	pyRun_String(code, pyFileInput, global, local)
	if pyErr_Occurred() {
		pyErr_Print()
		return "", ErrRunFailed
	}

	if item := pyDict_GetItemString(local, "_result"); item != 0 {
		value := pyUnicode_AsUTF8(item)
		return value, nil
	}

	return "", ErrNoResult
}

// checkInit checks if the Python interpreter has been initialized. It panics if it has not.
func checkInit() {
	if python == 0 {
		panic("serpent: Init must be called before Run")
	}
}
