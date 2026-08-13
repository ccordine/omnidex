package worker

func operationResultText(output map[string]any, key string) string {
	if output == nil {
		return ""
	}
	value, _ := output[key].(string)
	return value
}
