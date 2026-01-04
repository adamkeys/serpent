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
)

var (
	// ErrAlreadyInitialized is returned when the Python interpreter is initialized more than once.
	ErrAlreadyInitialized = errors.New("already initialized")
	// ErrRunFailed is returned when the Python program fails to run.
	ErrRunFailed = errors.New("run failed")
	// ErrSubInterpreterFailed is returned when a sub-interpreter fails to initialize.
	ErrSubInterpreterFailed = errors.New("sub-interpreter creation failed")
	// ErrNoHealthyWorkers is returned when all workers have failed.
	ErrNoHealthyWorkers = errors.New("no healthy workers available")
	// ErrNotInitialized is returned when Close is called before Init.
	ErrNotInitialized = errors.New("not initialized")
)

// PythonNotInitialized is a panic type indicating that the Python interpreter has not been initialized.
type PythonNotInitialized string

// Init initializes the Python interpreter with runtime.NumCPU() workers. This must be called before
// any other functions in this package. When using packages that are incompatible with sub-interpreters,
// use [InitSingleWorker] instead.
func Init(libraryPath string) error {
	supportsSubInterpreters, err := initPython(libraryPath)
	if err != nil {
		return err
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

// InitSingleWorker initializes the Python interpreter with a single worker, disabling sub-interpreters.
// Use this when running Python code that uses C extension modules incompatible with sub-interpreters.
// This must be called before any other functions in this package. Use [Init] for normal usage.
func InitSingleWorker(libraryPath string) error {
	if _, err := initPython(libraryPath); err != nil {
		return err
	}

	workerPool = &pool{
		workers: make([]*worker, 0, 1),
	}
	return initSingleWorker()
}

// Run runs a [Program] with the supplied argument and returns the result. The Python code must
// define a run() function that accepts the input and returns a JSON-serializable value.
//
// Example Python program:
//
//	def run(input):
//	    return input + 1
func Run[TInput, TResult any](program Program[TInput, TResult], arg TInput) (TResult, error) {
	exec, err := Load(program)
	if err != nil {
		return *new(TResult), err
	}
	defer exec.Close()
	return exec.Run(arg)
}

// RunWrite runs a [Program] with the supplied argument with the Python program writing to the supplied writer.
// The Python code must define a run() function that accepts the input and a writer object.
//
// Example Python program:
//
//	def run(input, writer):
//	    writer.write(b'OK')
func RunWrite[TInput any](w io.Writer, program Program[TInput, Writer], arg TInput) error {
	exec, err := LoadWriter(program)
	if err != nil {
		return err
	}
	defer exec.Close()
	return exec.Run(w, arg)
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

// Executable represents a loaded Python program that can be called multiple times.
// It is pinned to a single worker on first Run(), preserving module-level state across calls.
type Executable[TInput, TResult any] struct {
	executable
}

// Load loads a Python program and returns an Executable that can be called multiple times.
// The executable is pinned to a worker on first Run(), and all subsequent calls use the same worker.
func Load[TInput, TResult any](program Program[TInput, TResult]) (*Executable[TInput, TResult], error) {
	checkInit()
	return &Executable[TInput, TResult]{
		executable: executable{code: string(program)},
	}, nil
}

// Run executes the loaded program with the given input.
// On first call, the program is loaded on a worker and pinned to it.
// Subsequent calls reuse the same worker and loaded state.
func (e *Executable[TInput, TResult]) Run(arg TInput) (TResult, error) {
	input, err := json.Marshal(arg)
	if err != nil {
		return *new(TResult), fmt.Errorf("marshal input: %w", err)
	}

	if err := e.pin(); err != nil {
		return *new(TResult), err
	}

	result, err := e.runOnWorker(string(input))
	if err != nil {
		return *new(TResult), err
	}

	var value TResult
	if err := json.Unmarshal([]byte(result), &value); err != nil {
		return *new(TResult), fmt.Errorf("unmarshal result: %w", err)
	}

	return value, nil
}

// WriterExecutable represents a loaded Python program that writes to an output stream.
type WriterExecutable[TInput any] struct {
	executable
}

// LoadWriter loads a Python program that writes to an output stream.
func LoadWriter[TInput any](program Program[TInput, Writer]) (*WriterExecutable[TInput], error) {
	checkInit()
	return &WriterExecutable[TInput]{
		executable: executable{code: generateWriterCode(string(program))},
	}, nil
}

// Run executes the loaded program, writing output to the provided writer.
func (e *WriterExecutable[TInput]) Run(w io.Writer, arg TInput) error {
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
		pw.Close()
		wg.Wait()
		return fmt.Errorf("marshal input: %w", err)
	}

	if err := e.pin(); err != nil {
		pw.Close()
		wg.Wait()
		return err
	}

	_, err = e.runOnWorker(string(input))
	if err != nil {
		pw.Close()
		wg.Wait()
		return err
	}

	if err := pw.Close(); err != nil {
		wg.Wait()
		return fmt.Errorf("close writer: %w", err)
	}
	wg.Wait()

	return nil
}

// execContext identifies the context of an Executable run.
type execContext struct {
	exec  *execState
	input string

	cond *sync.Cond
	done bool

	value string
	err   error
}

// execute implements the request interface for execContext.
func (ctx *execContext) execute() {
	ctx.cond.L.Lock()
	defer func() {
		ctx.done = true
		ctx.cond.Signal()
		ctx.cond.L.Unlock()
	}()

	// Cleanup request (empty code signals cleanup)
	if ctx.exec.code == "" {
		if ctx.exec.globals != 0 {
			py_DecRef(ctx.exec.globals)
			ctx.exec.globals = 0
		}
		return
	}

	// Load the program if not already loaded
	if ctx.exec.globals == 0 {
		globals := pyDict_New()
		pyDict_SetItemString(globals, "__builtins__", pyEval_GetBuiltins())

		pyRun_String(ctx.exec.code, pyFileInput, globals, globals)
		if pyErr_Occurred() {
			ctx.err = fetchPythonError()
			py_DecRef(globals)
			return
		}

		ctx.exec.globals = globals
	}

	ctx.value, ctx.err = callRun(ctx.exec.globals, ctx.input)
}

// execState holds the loaded state of an Executable on a worker.
type execState struct {
	code    string
	globals pyObject
}

// executable holds common state and methods for Executable and WriterExecutable.
type executable struct {
	code   string
	worker *worker
	state  *execState
	mu     sync.Mutex
}

// pin assigns this executable to a worker if not already pinned.
func (b *executable) pin() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.worker == nil {
		if workerPool == nil || len(workerPool.workers) == 0 {
			return ErrNoHealthyWorkers
		}
		idx := workerPool.next.Add(1) % uint64(len(workerPool.workers))
		b.worker = workerPool.workers[idx]
		b.state = &execState{code: b.code}
	}
	return nil
}

// runOnWorker sends a request to the pinned worker.
func (b *executable) runOnWorker(input string) (string, error) {
	if b.worker.initErr != nil {
		return "", fmt.Errorf("%w: %v", ErrSubInterpreterFailed, b.worker.initErr)
	}

	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	cond.L.Lock()
	defer cond.L.Unlock()

	ctx := &execContext{
		exec:  b.state,
		input: input,
		cond:  cond,
	}
	b.worker.requests <- ctx
	for !ctx.done {
		cond.Wait()
	}

	return ctx.value, ctx.err
}

// Close releases resources associated with the executable.
func (b *executable) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state != nil && b.worker != nil {
		var mu sync.Mutex
		cond := sync.NewCond(&mu)
		cond.L.Lock()
		defer cond.L.Unlock()

		ctx := &execContext{
			exec:  b.state,
			input: "",
			cond:  cond,
		}
		ctx.exec.code = ""
		b.worker.requests <- ctx
		for !ctx.done {
			cond.Wait()
		}
	}

	b.worker = nil
	b.state = nil
	return nil
}
