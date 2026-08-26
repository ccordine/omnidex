package worker

func phpServiceStateShapeMethodsSource() string {
	return `    public static function assertShape(array $value, array $schema): void
    {
        foreach ($value as $field => $fieldValue) {
            if (!is_string($field) || !array_key_exists($field, $schema)) {
                throw new InvalidArgumentException('Durable state contains an unknown interface field.');
            }
            $definition = $schema[$field];
            if (!is_array($definition) || !isset($definition['kind']) ||
                !is_string($definition['kind']) || !isset($definition['fields']) ||
                !is_array($definition['fields'])) {
                throw new RuntimeException('Durable state interface definition is invalid.');
            }
            self::assertField($fieldValue, $definition['kind'], $definition['fields']);
        }
    }

    private static function assertField(mixed $value, string $kind, array $recordFields): void
    {
        $valid = match ($kind) {
            'string' => is_string($value),
            'integer' => is_int($value),
            'number' => is_int($value) || is_float($value),
            'boolean' => is_bool($value),
            'string_list' => self::isScalarList($value, 'string'),
            'integer_list' => self::isScalarList($value, 'integer'),
            'number_list' => self::isScalarList($value, 'number'),
            'boolean_list' => self::isScalarList($value, 'boolean'),
            'record_list' => self::isRecordList($value, $recordFields),
            default => throw new RuntimeException('Durable state interface kind is unsupported.'),
        };
        if (!$valid) {
            throw new InvalidArgumentException('Durable state value violates its exact interface.');
        }
    }

    private static function isScalarList(mixed $value, string $kind): bool
    {
        if (!is_array($value) || !array_is_list($value)) {
            return false;
        }
        foreach ($value as $item) {
            $valid = match ($kind) {
                'string' => is_string($item),
                'integer' => is_int($item),
                'number' => is_int($item) || is_float($item),
                'boolean' => is_bool($item),
                default => false,
            };
            if (!$valid) {
                return false;
            }
        }
        return true;
    }

    private static function isRecordList(mixed $value, array $fields): bool
    {
        if (!is_array($value) || !array_is_list($value)) {
            return false;
        }
        foreach ($value as $record) {
            if (!is_array($record)) {
                return false;
            }
            foreach ($record as $name => $fieldValue) {
                if (!is_string($name) || !isset($fields[$name]) ||
                    !is_string($fields[$name])) {
                    return false;
                }
                self::assertField($fieldValue, $fields[$name], []);
            }
        }
        return true;
    }`
}
