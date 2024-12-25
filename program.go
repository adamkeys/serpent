package serpent

import (
	"strconv"
	"strings"
)

// Writer is a result type which indicates that the program writes to the output.
// e.g. Program[string, Writer] is a program that writes to the output.
type Writer struct{}

// Program identifies a Python program.
type Program[TInput, TResult any] string

// generateCode generates the Python code for the program.
func generateCode(code string, input []byte) string {
	var builder strings.Builder
	builder.WriteString("import json\n")
	builder.WriteString("input = json.loads(")
	builder.WriteString(strconv.Quote(string(input)))
	builder.WriteString(")\n")
	builder.WriteString(code)
	builder.WriteString(`
try:
	_result = json.dumps(result)
except:
	pass
`)
	return builder.String()
}

// generateCode generates the Python code for the program.
func generateWriterCode(code string, input []byte) string {
	var builder strings.Builder
	builder.WriteString("fd = input['Fd']\n")
	builder.WriteString("input = input['Input']\n")
	builder.WriteString(code)

	return generateCode(builder.String(), input)
}
