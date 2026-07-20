package mapper

func cloneStrings(source []string) []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source...)
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
