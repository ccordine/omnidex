package worker

import "fmt"

func laravelStateRuntimeAPI() string {
	return phpServiceStateRuntimeAPI()
}

func laravelStateRuntimeSource() string {
	return fmt.Sprintf(`final class RuntimeState
{
    private static bool $schemaVerified = false;
	private static array $revisions = [];

    public static function load(string $scope, string $key): ?array
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        self::assertSchema();
		$row = Illuminate\Support\Facades\DB::table(%s)
			->where('state_scope', $scope)->where('state_key', $key)
			->first(['state_value', 'revision']);
		$identity = self::identity($scope, $key);
		if ($row === null) {
			self::$revisions[$identity] = 0;
            return null;
        }
		$revision = filter_var($row->revision ?? null, FILTER_VALIDATE_INT,
			['options' => ['min_range' => 1]]);
		if ($revision === false) {
			throw new RuntimeException('Durable service-state revision is invalid.');
		}
		$encoded = $row->state_value ?? null;
        if (is_array($encoded)) {
			self::$revisions[$identity] = $revision;
            return $encoded;
        }
        if (!is_string($encoded)) {
            throw new RuntimeException('Durable service state has an unsupported database value.');
        }
        $value = json_decode($encoded, true, 512, JSON_THROW_ON_ERROR);
        if (!is_array($value)) {
            throw new RuntimeException('Durable service state must decode to an array.');
        }
		self::$revisions[$identity] = $revision;
        return $value;
    }

    public static function save(string $scope, string $key, array $value): void
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        self::assertSchema();
		$identity = self::identity($scope, $key);
		if (!array_key_exists($identity, self::$revisions)) {
			throw new RuntimeException('Durable service state must be loaded before it is saved.');
		}
		$revision = self::$revisions[$identity];
		$encoded = json_encode($value, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
		if ($revision === 0) {
			$changed = Illuminate\Support\Facades\DB::affectingStatement(
				'INSERT INTO %s (state_scope, state_key, state_value) ' .
				'VALUES (?, ?, CAST(? AS JSONB)) ON CONFLICT DO NOTHING',
				[$scope, $key, $encoded],
			);
		} else {
			$changed = Illuminate\Support\Facades\DB::affectingStatement(
				'UPDATE %s SET state_value = CAST(? AS JSONB), revision = revision + 1, updated_at = NOW() ' .
				'WHERE state_scope = ? AND state_key = ? AND revision = ?',
				[$encoded, $scope, $key, $revision],
			);
		}
        if ($changed !== 1) {
			throw new RuntimeException('Durable service-state write lost optimistic revision authority.');
        }
		self::$revisions[$identity] = $revision + 1;
    }

    public static function delete(string $scope, string $key): bool
    {
        self::validateKey('scope', $scope);
        self::validateKey('key', $key);
        self::assertSchema();
        $changed = Illuminate\Support\Facades\DB::table(%s)
            ->where('state_scope', $scope)->where('state_key', $key)->delete();
        if ($changed > 1) {
            throw new RuntimeException('Durable service-state delete changed an unexpected row count.');
        }
		unset(self::$revisions[self::identity($scope, $key)]);
        return $changed === 1;
    }

%s

	private static function identity(string $scope, string $key): string
	{
		return $scope . "\0" . $key;
	}

    private static function assertSchema(): void
    {
        if (self::$schemaVerified) {
            return;
        }
        $version = Illuminate\Support\Facades\DB::table(%s)->where('singleton', true)->value('version');
        if ($version === null || (int) $version !== %d) {
            throw new RuntimeException('Durable service-state schema version is missing or unsupported.');
        }
        self::$schemaVerified = true;
    }

    private static function validateKey(string $label, string $value): void
    {
        if ($value === '' || strlen($value) > 200 || preg_match('/[\x00-\x1F\x7F]/', $value) === 1) {
            throw new InvalidArgumentException('Durable service-state ' . $label . ' is invalid.');
        }
    }
	}`, phpSingleQuoted(directCodingServiceStateRecordTable), directCodingServiceStateRecordTable,
		directCodingServiceStateRecordTable, phpSingleQuoted(directCodingServiceStateRecordTable),
		phpServiceStateShapeMethodsSource(),
		phpSingleQuoted(directCodingServiceStateSchemaTable),
		directCodingServiceStateSchemaVersion)
}

func laravelHTTPRuntimeSource() string {
	return `final class LaravelHttpFailure extends RuntimeException
{
    public function __construct(public readonly int $status, string $message)
    {
        parent::__construct($message);
    }
}

final class LaravelRuntime
{
    public static function taskInput(
        Illuminate\Http\Request $request,
        string $requestMedia,
    ): TaskInput {
        $body = $request->getContent();
        if (!is_string($body)) {
            throw new LaravelHttpFailure(400, 'Request body could not be read.');
        }
        $contentType = strtolower(trim(explode(';', (string) $request->header('content-type', ''))[0]));
        if ($requestMedia !== 'none' && $contentType !== $requestMedia) {
            throw new LaravelHttpFailure(415, 'Request media type does not match the endpoint contract.');
        }
        $payload = null;
        switch ($requestMedia) {
            case 'none':
                break;
            case 'application/json':
                try {
                    $payload = json_decode($body, true, 512, JSON_THROW_ON_ERROR);
                } catch (JsonException $failure) {
                    throw new LaravelHttpFailure(400, 'Request body is not valid JSON.');
                }
                break;
            case 'application/x-www-form-urlencoded':
                parse_str($body, $payload);
                break;
            case 'multipart/form-data':
                $payload = ['fields' => $request->request->all(), 'files' => []];
                break;
            case 'application/xml':
            case 'text/plain':
                $payload = $body;
                break;
            default:
                throw new LogicException('Request media type is not registered.');
        }
        $headers = [];
        foreach ($request->headers->all() as $name => $values) {
            if (is_string($name) && is_array($values) && isset($values[0]) && is_string($values[0])) {
                $headers[strtolower($name)] = $values[0];
            }
        }
        return new TaskInput(
            strtoupper($request->method()), '/' . ltrim($request->path(), '/'),
            $request->route()?->parameters() ?? [], $request->query->all(),
            $headers, $body, $payload,
        );
    }

    public static function response(
        TaskResult $result,
        string $media,
        int $status,
        ?string $html = null,
    ): Symfony\Component\HttpFoundation\Response {
        RuntimeAssertions::requireResult($result);
        if ($result->error !== '') {
            return response($result->error, 422, ['Content-Type' => 'text/plain']);
        }
        if ($status === 204) {
            if ($html !== null) {
                throw new LogicException('A no-content response cannot carry HTML.');
            }
            return response('', 204);
        }
        if ($media === 'text/html') {
            if ($html === null || trim($html) === '') {
                throw new LogicException('HTML response requires one rendered representation.');
            }
            return response($html, $status, ['Content-Type' => 'text/html']);
        }
        if ($html !== null) {
            throw new LogicException('A non-HTML response cannot carry an HTML representation.');
        }
        switch ($media) {
            case 'application/json':
                $body = json_encode($result, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
                break;
            case 'application/xml':
                $body = '<result><output>' . self::escape($result->output) . '</output><state>' .
                    self::escape(json_encode($result->state, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES)) .
                    '</state></result>';
                break;
            case 'text/plain':
                $body = $result->output;
                break;
            default:
                throw new LogicException('Response media type is not registered.');
        }
        return response($body, $status, ['Content-Type' => $media]);
    }

    public static function failure(Throwable $failure): Symfony\Component\HttpFoundation\Response
    {
        $status = $failure instanceof LaravelHttpFailure ? $failure->status : 500;
        if ($status >= 500) {
            error_log('Unhandled Laravel route failure [' . get_class($failure) . ']: ' . $failure->getMessage());
            report($failure);
            return response('Internal service failure.', $status, ['Content-Type' => 'text/plain']);
        }
        return response($failure->getMessage(), $status, ['Content-Type' => 'text/plain']);
    }

    private static function escape(string $value): string
    {
        return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
    }
}`
}
