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

    program := serpent.Program[int, int]("result = input * 2")
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

Your Python code has access to:

- `input` - The input value (serialized as JSON)
- `result` - Set this variable to return a value to Go (will be serialized as JSON)
- `fd` - A file descriptor for writing output (when using `RunWrite`)

## Python Code Guidelines

### Returning Values

Your Python code must set a variable named `result` for the output:

```python
# Python receives 'input' and must set 'result'
result = input.upper()
```

### Writing Output

When using `RunWrite`, Python has access to a file descriptor `fd`:

```python
import os

os.write(fd, f"Hello from Python: {input}\n".encode())
os.close(fd)
```

### Using External Libraries

Python code can import any library available in the Python environment:

```python
from transformers import pipeline

ner = pipeline("ner", grouped_entities=True)
entities = ner(input)
result = [e["word"] for e in entities]
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
