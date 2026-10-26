package homeassistant

// ValidateService uses the same service-path contract as outbound dispatch.
func ValidateService(service string) error {
	_, _, err := parseService(service)
	return err
}
