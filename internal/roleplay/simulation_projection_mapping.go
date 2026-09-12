package roleplay

import "slices"

func reverseSimulationSlice[T any](values []T) []T {
	slices.Reverse(values)
	return values
}

func meterKeys(values []MeterProjection) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Key
	}
	return result
}

func inventoryIDs(values []InventoryItemProjection) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}

func canonTexts(values []ContextFact) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Content
	}
	return result
}

func canonIDs(values []ContextFact) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.EventID
	}
	return result
}

func memoryTexts(values []CharacterMemory) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Content
	}
	return result
}

func memoryIDs(values []CharacterMemory) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}
