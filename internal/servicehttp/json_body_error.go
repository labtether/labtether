package servicehttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
)

// JSONBodyErrorAnswer says, as a status and words, why a request body could
// not be read as the JSON object a route takes.
func JSONBodyErrorAnswer(err error) (int, string) {
	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge, fmt.Sprintf("The request body is larger than %s.", byteWords(tooLarge.Limit))
	case errors.Is(err, io.EOF):
		return http.StatusBadRequest, "The request needs a JSON body."
	case errors.As(err, &typeErr) && typeErr.Field != "" && typeErr.Type != nil:
		return http.StatusBadRequest, fmt.Sprintf("%s must be %s.", typeErr.Field, jsonKindWords(typeErr.Type.Kind()))
	default:
		return http.StatusBadRequest, "The request body is not a valid JSON object."
	}
}

// WriteJSONBodyError answers a request whose body could not be read. A route
// that returned without answering sent an empty 200, so the caller believed
// the request had worked.
func WriteJSONBodyError(w http.ResponseWriter, err error) {
	status, message := JSONBodyErrorAnswer(err)
	WriteError(w, status, message)
}

func byteWords(n int64) string {
	if n >= 1<<20 && n%(1<<20) == 0 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}

func jsonKindWords(kind reflect.Kind) string {
	switch kind {
	case reflect.String:
		return "text"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	default:
		return "an object"
	}
}
