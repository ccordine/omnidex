package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func phpServiceStateInterfaceAPI(
	className string,
	binding directCodingServiceStateInterfaceBinding,
	writable bool,
) string {
	methods := "  public static function load(): array;"
	if writable {
		methods += "\n  public static function save(array $value): void;"
	}
	return fmt.Sprintf("final class %s {\n%s\n}\n%s", className, methods,
		phpServiceStateInterfaceSummary(binding))
}

func phpServiceStateInterfaceSummary(
	binding directCodingServiceStateInterfaceBinding,
) string {
	fields := make([]string, 0, len(binding.Result.Fields))
	for _, field := range binding.Result.Fields {
		description := field.Name + " (" + field.Purpose + "):" + string(field.Kind)
		if field.Kind == assemblyline.ApplicationServiceStateRecordList {
			records := make([]string, 0, len(field.RecordFields))
			for _, record := range field.RecordFields {
				records = append(
					records,
					record.Name+" ("+record.Purpose+"):"+string(record.Kind),
				)
			}
			description += "{" + strings.Join(records, ",") + "}"
		}
		fields = append(fields, description)
	}
	return "Exact shared durable state fields: " + strings.Join(fields, "; ") +
		". Unknown fields and incompatible values fail."
}

func phpServiceStateInterfaceSchemaSource(
	result assemblyline.ApplicationServiceStateInterfaceResult,
) string {
	fields := make([]string, 0, len(result.Fields))
	for _, field := range result.Fields {
		recordFields := make([]string, 0, len(field.RecordFields))
		for _, record := range field.RecordFields {
			recordFields = append(recordFields, fmt.Sprintf(
				"%s => %s", phpSingleQuoted(record.Name), phpSingleQuoted(string(record.Kind)),
			))
		}
		fields = append(fields, fmt.Sprintf(
			"%s => ['kind' => %s, 'fields' => [%s]]",
			phpSingleQuoted(field.Name), phpSingleQuoted(string(field.Kind)),
			strings.Join(recordFields, ", "),
		))
	}
	return "[" + strings.Join(fields, ", ") + "]"
}

func phpServiceStateInterfaceFacadeSource(
	className string,
	namespace string,
	binding directCodingServiceStateInterfaceBinding,
	writable bool,
) string {
	save := ""
	if writable {
		save = `

    public static function save(array $value): void
    {
        RuntimeState::assertShape($value, self::SCHEMA);
        $root = RuntimeState::load(self::SCOPE, self::KEY);
        if ($root === null) {
            $root = [];
        }
        $root[self::INTERFACE_KEY] = $value;
        RuntimeState::save(self::SCOPE, self::KEY, $root);
    }`
	}
	return fmt.Sprintf(`final class %s
{
    private const SCOPE = %s;
    private const KEY = %s;
    private const INTERFACE_KEY = %s;
    private const SCHEMA = %s;

    public static function load(): array
    {
        $root = RuntimeState::load(self::SCOPE, self::KEY);
        if ($root === null || !array_key_exists(self::INTERFACE_KEY, $root)) {
            return [];
        }
        $value = $root[self::INTERFACE_KEY];
        if (!is_array($value)) {
            throw new RuntimeException('Shared durable state interface value is not an object.');
        }
        RuntimeState::assertShape($value, self::SCHEMA);
        return $value;
    }%s

}`, className, phpSingleQuoted(namespace), phpSingleQuoted(directCodingServiceStateDefaultKey),
		phpSingleQuoted(binding.ID), phpServiceStateInterfaceSchemaSource(binding.Result), save)
}
