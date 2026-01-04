# Serpent 🐍

A Go library for embedding and running Python code directly in your Go applications with high performance and concurrency support.

## Features

- **Embed Python code** directly in Go using `go:embed`
- **Type-safe API** with Go generics for input/output
- **Concurrent execution** with a pool of sub-interpreters
- **Automatic Python library discovery** on macOS, Linux, and other Unix systems
- **Multiple execution modes**: return values, write to streams, or use custom I/O

## Installation

```bash
go get github.com/adamkeys/serpent
```

### Requirements

- Go 1.18 or later
- Python 3 shared library installed on your system

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "github.com/adamkeys/serpent"
)

func main() {
    lib, err := serpent.Lib()
    if err != nil {
        log.Fatal(err)
    }
    if err := serpent.Init(lib); err != nil {
        log.Fatal(err)
    }

    program := serpent.Program[int, int]("def run(input): return input * 2")
    result, err := serpent.Run(program, 21)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result) // Output: 42
}
```

See the [examples/](examples/) directory for more complete demonstrations including concurrent execution, embedding Python files, and using external libraries like HuggingFace Transformers.

## API Reference

### Initialization

- **`Lib() (string, error)`** - Automatically discovers the Python shared library path
- **`Init(libPath string) error`** - Initializes the Python interpreter with a worker pool
- **`InitSingleWorker(libPath string) error`** - Initializes with a single worker (for libraries that don't support sub-interpreters)
- **`Close() error`** - Cleans up and shuts down the interpreter

### Execution

- **`Run[I, O](program Program[I, O], input I) (O, error)`** - Executes Python code and returns the result
- **`RunWrite[I](w io.Writer, program Program[I, Writer], input I) error`** - Executes Python code that writes to a Go io.Writer

### Program Definition

A `Program[I, O]` is simply a string containing Python code:

```go
type Program[I, O any] string
```

Your Python code should define a `run` function:

- **`def run(input):`** - For `Run`, return the result value
- **`def run(input, writer):`** - For `RunWrite`, write to the provided writer

## Python Code Guidelines

### Returning Values

Define a `run` function that takes input and returns the result:

```python
def run(input):
    return input.upper()
```

### Writing Output

When using `RunWrite`, your `run` function receives a `writer` object:

```python
def run(input, writer):
    writer.write(f"Hello from Python: {input}\n")
```

The `writer` object provides:

- **`write(data)`** - Write string or bytes (strings are auto-encoded as UTF-8)
- **`flush()`** - Flush the output (no-op, writes are unbuffered)

The writer is automatically closed when your function returns.

### Using External Libraries

Python code can import any library available in the Python environment:

```python
from transformers import pipeline

ner = pipeline("ner", grouped_entities=True)

def run(input):
    entities = ner(input)
    return [e["word"] for e in entities]
```

**Note**: Libraries that don't support sub-interpreters require initialization with `InitSingleWorker()` instead of `Init()`.

## Examples

The [examples/](examples/) directory contains several demonstrations:

- **[hello/](examples/hello/)** - Basic concurrent "Hello World" example
- **[identity/](examples/identity/)** - Simple identity transformation
- **[transformers/](examples/transformers/)** - Named entity recognition using HuggingFace Transformers

## Environment Variables

- **`LIBPYTHON_PATH`** - Override automatic library discovery by specifying the Python shared library path directly

## How It Works

Serpent uses [purego](https://github.com/ebitengine/purego) to dynamically load and call Python's C API without CGO. It manages a pool of Python sub-interpreters (each running on its own OS thread) to enable safe concurrent execution of Python code from multiple goroutines.

Input and output values are serialized as JSON, providing a simple and type-safe interface between Go and Python.

## Platform Support

- ✅ macOS (Darwin)
- ✅ Linux
- ✅ Unix-like systems
