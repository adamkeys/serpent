package serpent

import "encoding/json"

// Program identifies a Python program.
type Program[TInput, TResult any] string

// Functions that implement the [programmer] interface.
func (p Program[TInput, TResult]) getCode() string { return string(p) }
func (p Program[TInput, TResult]) transformInput(value any) ([]byte, error) {
	return json.Marshal(value)
}
func (p Program[TInput, TResult]) transformOutput(data []byte) (any, error) {
	return unmarshalJSON[TResult](data)
}

// unmarshalJSON unmarshals the JSON byte slice into the result of type TResult.
func unmarshalJSON[TResult any](data []byte) (any, error) {
	var result TResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}
