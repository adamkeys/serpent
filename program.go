package serpent

import "strings"

// Writer is a result type which indicates that the program writes to the output.
// e.g. Program[string, Writer] is a program that writes to the output.
type Writer struct{}

// Program identifies a Python program.
type Program[TInput, TResult any] string

// writerClassDef is the Python code for the Writer class injected into writer programs.
const writerClassDef = `
import os

class Writer:
    def __init__(self, fd):
        self._fd = fd
        self._closed = False

    def write(self, data):
        if self._closed:
            raise RuntimeError("Writer is closed")
        if isinstance(data, str):
            data = data.encode('utf-8')
        os.write(self._fd, data)

    def flush(self):
        pass

    def close(self):
        if not self._closed:
            os.close(self._fd)
            self._closed = True

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
        return False
`

// writerRunWrapper is the Python code that wraps the user's run() function to support
// the Writer type when using RunWrite.
const writerRunWrapper = `
_user_run = run
def run(raw_input):
    import os
    _input = raw_input['Input']
    _fd = os.dup(raw_input['Fd'])
    _writer = Writer(_fd)
    try:
        _user_run(_input, _writer)
    finally:
        _writer.close()
    return None
`

// generateWriterCode generates Python code for programs that write to an output stream.
// It injects the Writer class definition and wraps the user's run() function to handle
// the writer setup and teardown.
func generateWriterCode(code string) string {
	var builder strings.Builder
	builder.WriteString(writerClassDef)
	builder.WriteString("\n")
	builder.WriteString(code)
	builder.WriteString("\n")
	builder.WriteString(writerRunWrapper)
	return builder.String()
}
