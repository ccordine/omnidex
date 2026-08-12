package cognitionreplay

func NewCanonicalJSONBlob(value any) (Blob, error) {
	raw, err := marshalCanonical(value)
	if err != nil {
		return Blob{}, err
	}
	return NewBlob("application/json", raw)
}
