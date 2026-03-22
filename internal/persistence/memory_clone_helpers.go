package persistence

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	out := make([]string, len(input))
	copy(out, input)
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneAnyValue(value)
	}
	return cloned
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for idx := range typed {
			out[idx] = cloneAnyValue(typed[idx])
		}
		return out
	case []string:
		return cloneStringSlice(typed)
	case map[string]string:
		return cloneMetadata(typed)
	default:
		return value
	}
}

func cloneFloatPtr(input *float64) *float64 {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}
