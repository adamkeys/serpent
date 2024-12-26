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
var py_Release func()
var pyGILState_Ensure func() pyThreadState
var pyGILState_Release func(pyThreadState)
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
	// ErrAlreadyInitialized is returned when the Python interpreter is initialized more than once.
	ErrAlreadyInitialized = errors.New("already initialized")
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
	if python != 0 {
		return ErrAlreadyInitialized
	}

	lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("dlopen: %v", err)
	}
	python = lib

	purego.RegisterLibFunc(&py_InitializeEx, python, "Py_InitializeEx")
	purego.RegisterLibFunc(&py_Release, python, "Py_Finalize")
	purego.RegisterLibFunc(&pyGILState_Ensure, python, "PyGILState_Ensure")
	purego.RegisterLibFunc(&pyGILState_Release, python, "PyGILState_Release")
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

	go start()

	return nil
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

// RunFd runs a [Program] with the supplied argument with the Python program writing to the supplied writer.
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
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	cond.L.Lock()
	defer cond.L.Unlock()

	ctx := &runContext{code: code, cond: cond}
	runCh <- ctx
	for !ctx.done {
		cond.Wait()
	}

	return ctx.value, ctx.err
}

// runCh is a channel for sending Python code to the Python interpreter.
var runCh = make(chan *runContext, 1)

// start runs a loop waiting for instructions.
func start() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	py_InitializeEx(0)
	defer py_Release()

	for run := range runCh {
		run.cond.L.Lock()

		global := pyDict_New()
		local := pyDict_New()

		pyRun_String(run.code, pyFileInput, global, local)
		if pyErr_Occurred() {
			pyErr_Print()
			run.err = ErrRunFailed
		} else if item := pyDict_GetItemString(local, "_result"); item != 0 {
			run.value = pyUnicode_AsUTF8(item)
		} else {
			run.err = ErrNoResult
		}

		// This is a good candidate for sending the result on a channel, but doing so conflicts with Python's GIL.
		// To work around that we set the result on the context and signal that the run is complete. The calling
		// Run function waits for changes on the done state to know when the result is ready.
		run.done = true
		run.cond.Signal()
		run.cond.L.Unlock()

		py_DecRef(local)
		py_DecRef(global)
	}
}

// checkInit checks if the Python interpreter has been initialized. It panics if it has not.
func checkInit() {
	if python == 0 {
		panic("serpent: Init must be called before Run")
	}
}
