// Package serpent provides functions for interacting with a Python interpreter.
package serpent

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/ebitengine/purego"
)

// Types used in the Python C API.
type pyObject uintptr
type pyThreadState uintptr

// Function prototypes for the Python C API.
var py_Initialize func()
var py_Finalize func()
var pyEval_InitThreads func()
var pyErr_Occurred func() bool
var pyErr_Print func()
var pyObject_Type func(pyObject) pyObject
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

// Constants used in the Python C API.
const pyFileInput = 257

var (
	// python is a handle to the Python shared library.
	python uintptr
	// execCh is a channel for receiving Python programs.
	execCh = make(chan execContext)
)

// Init initializes the Python interpreter, loading the Python shared library from the supplied path. This
// must be called before any other functions in this package.
func Init(libraryPath string) error {
	lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("dlopen: %v", err)
	}
	python = lib
	execCh = make(chan execContext)

	purego.RegisterLibFunc(&py_Initialize, python, "Py_Initialize")
	purego.RegisterLibFunc(&py_Finalize, python, "Py_Finalize")
	purego.RegisterLibFunc(&pyErr_Occurred, python, "PyErr_Occurred")
	purego.RegisterLibFunc(&pyErr_Print, python, "PyErr_Print")
	purego.RegisterLibFunc(&pyObject_Type, python, "PyObject_Type")
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

	py_Initialize()
	go start()

	return nil
}

// Run runs a [Program] with the supplied argument and returns the result. The Python code must assign
// a result variable in the main program.
func Run[TInput, TResult any](program Program[TInput, TResult], arg TInput) (TResult, error) {
	if python == 0 {
		panic("serpent: Init must be called before Run")
	}

	resultCh := make(chan result)
	execCh <- execContext{
		program: program,
		input:   arg,
		result:  resultCh,
	}
	result := <-resultCh
	if result.err != nil {
		return *new(TResult), result.err
	}
	return result.value.(TResult), nil
}

// programmer identifies a Python program.
type programmer interface {
	// getCode returns the code for the program.
	getCode() string
	// transformInput transforms the input into a JSON byte slice.
	transformInput(value any) ([]byte, error)
	// transformOutput transforms the JSON byte slice into the output.
	transformOutput(data []byte) (any, error)
}

// result identifies the result of a program execution. It contains the result and an error if one occurred.
type result struct {
	value any
	err   error
}

// execContext identifies an execution context.
type execContext struct {
	program programmer
	input   any
	result  chan<- result
}

// start starts the Python interpreter in a loop executing each program as they are received from the channel.
func start() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for ctx := range execCh {
		program := ctx.program
		input, err := program.transformInput(ctx.input)
		if err != nil {
			ctx.result <- result{err: err}
			continue
		}

		var main strings.Builder
		main.WriteString("import json\n")
		main.WriteString("input = json.loads('")
		main.Write(input)
		main.WriteString("')\n")
		main.WriteString(program.getCode())
		main.WriteString("\n_result = json.dumps(result)")

		func() {
			global := pyDict_New()
			defer py_DecRef(global)
			local := pyDict_New()
			defer py_DecRef(local)

			var result result
			pyRun_String(main.String(), pyFileInput, global, local)
			if pyErr_Occurred() {
				pyErr_Print()
				result.err = fmt.Errorf("run failed")
			} else {
				item := pyDict_GetItemString(local, "_result")
				data := pyUnicode_AsUTF8(item)
				result.value, result.err = program.transformOutput([]byte(data))
			}

			ctx.result <- result
		}()
	}
}
