package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func phpServiceRuntimeDocument(
	hasHTML, hasState bool,
	routeBlocks []assemblyline.SourceBlock,
) assemblyline.SourceDocument {
	blocks := []assemblyline.SourceBlock{
		{ID: "runtime.task_input", Static: phpServiceTaskInputSource(), API: phpServiceTaskInputAPI()},
		{ID: "runtime.task_result", Static: phpServiceTaskResultSource(), API: phpServiceTaskResultAPI()},
	}
	if hasState {
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: "runtime.state", Static: phpServiceStateRuntimeSource(),
			API: phpServiceStateRuntimeAPI(),
		})
	}
	if hasHTML {
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: "runtime.html", Static: phpServiceHTMLRuntimeSource(),
			API: phpServiceHTMLRuntimeAPI(), DependsOn: []string{"runtime.task_result"},
		})
	}
	blocks = append(blocks, routeBlocks...)
	blocks = append(blocks,
		assemblyline.SourceBlock{
			ID: "runtime.assertions", Static: phpServiceAssertionsSource(),
			API: phpServiceAssertionsAPI(), DependsOn: []string{"runtime.task_result"},
		},
		assemblyline.SourceBlock{
			ID: "runtime.http", Static: phpServiceHTTPRuntimeSource(),
			API: "code-owned HTTP request decoding, routing, rendering, and response emission",
			DependsOn: []string{
				"runtime.task_input", "runtime.task_result", "runtime.assertions",
			},
		},
	)
	return assemblyline.SourceDocument{
		ID: "application_runtime", Path: "src/Runtime.php", AdapterID: phpSourceAdapterID,
		Preamble: "<?php\ndeclare(strict_types=1);",
		Blocks:   blocks,
	}
}

func phpServiceRuntimeAPI() string {
	return phpServiceTaskInputAPI() + "\n" + phpServiceTaskResultAPI() + "\n" + phpServiceAssertionsAPI()
}

func phpServiceTaskInputAPI() string {
	return `final readonly class TaskInput {
  public string $method;
  public string $route;
  public array $routeParameters;
  public array $query;
  public array $headers;
  public string $body;
  public mixed $payload;
}`
}

func phpServiceTaskResultAPI() string {
	return `final readonly class TaskResult implements JsonSerializable {
  public static function success(string $output = '', array $state = []): self;
  public static function failure(string $error, array $state = []): self;
  public string $output;
  public string $error;
  public array $state;
}`
}

func phpServiceAssertionsAPI() string {
	return `final class RuntimeAssertions {
  public static function requireResult(TaskResult $result): void;
  public static function require(TaskResult $result, bool $condition, string $failure): void;
}`
}

func phpServiceHTMLRuntimeAPI() string {
	return `final readonly class RuntimeRoute {
  public string $path;
  public string $method;
}
final class RuntimeHtml {
  public static function document(string $title, string $body): string;
  public static function escape(string $value): string;
  public static function state(array $value): string;
  public static function records(array $state, string $key): array;
  public static function field(array $record, string $key): string;
  public static function formOpen(RuntimeRoute $route): string;
  public static function formClose(): string;
  public static function link(RuntimeRoute $route, string $label): string;
}`
}

func phpServiceRuntimeSource() string {
	return phpServiceTaskInputSource() + "\n\n" + phpServiceTaskResultSource() + "\n\n" +
		phpServiceHTMLRuntimeSource() + "\n\n" + phpServiceAssertionsSource() + "\n\n" +
		phpServiceHTTPRuntimeSource()
}

func phpServiceHTMLRuntimeSource() string {
	return `final readonly class RuntimeRoute
{
    public string $path;
    public string $method;

    public function __construct(string $path, string $method)
    {
        if ($path === '' || $path[0] !== '/' || str_contains($path, "\0") ||
            str_contains($path, "\r") || str_contains($path, "\n")) {
            throw new InvalidArgumentException('Runtime route path is invalid.');
        }
        $normalizedMethod = strtoupper($method);
        if (!in_array($normalizedMethod, ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'], true)) {
            throw new InvalidArgumentException('Runtime route method is unsupported.');
        }
        $this->path = $path;
        $this->method = $normalizedMethod;
    }
}

final class RuntimeHtml
{
    public static function document(string $title, string $body): string
    {
        if (trim($title) === '' || trim($body) === '') {
            throw new InvalidArgumentException('HTML representation requires a title and body.');
        }
        if (stripos($body, '<script') !== false || stripos($body, 'javascript:') !== false ||
            preg_match('/<[a-z][^>]*\\son[a-z]+\\s*=/i', $body) === 1) {
            throw new InvalidArgumentException('HTML representation contains executable browser content.');
        }
        return '<!doctype html><html lang="en"><head><meta charset="utf-8">' .
            '<meta name="viewport" content="width=device-width,initial-scale=1">' .
            '<link rel="stylesheet" href="/assets/app.css"><title>' . self::escape($title) .
            '</title></head><body>' . $body . '</body></html>';
    }

    public static function escape(string $value): string
    {
        return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
    }

    public static function state(array $value): string
    {
        return self::escape(json_encode($value, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES));
    }

    public static function records(array $state, string $key): array
    {
        if ($key === '' || !array_key_exists($key, $state) || !is_array($state[$key])) {
            throw new InvalidArgumentException('HTML record collection is absent or invalid.');
        }
        foreach ($state[$key] as $record) {
            if (!is_array($record)) {
                throw new InvalidArgumentException('HTML record collection contains a non-record value.');
            }
        }
        return array_values($state[$key]);
    }

    public static function field(array $record, string $key): string
    {
        if ($key === '' || !array_key_exists($key, $record)) {
            throw new InvalidArgumentException('HTML record field is absent.');
        }
        $value = $record[$key];
        if (is_string($value) || is_int($value) || is_float($value)) {
            return (string) $value;
        }
        if (is_bool($value)) {
            return $value ? 'true' : 'false';
        }
        throw new InvalidArgumentException('HTML record field is not scalar.');
    }

    public static function formOpen(RuntimeRoute $route): string
    {
        $browserMethod = $route->method === 'GET' ? 'get' : 'post';
        $override = $route->method === 'GET' || $route->method === 'POST' ? '' :
            '<input type="hidden" name="_method" value="' . self::escape($route->method) . '">';
        return '<form action="' . self::escape($route->path) . '" method="' . $browserMethod . '">' . $override;
    }

    public static function formClose(): string
    {
        return '</form>';
    }

    public static function link(RuntimeRoute $route, string $label): string
    {
        if ($route->method !== 'GET' || trim($label) === '') {
            throw new InvalidArgumentException('HTML link requires one GET route and label.');
        }
        return '<a href="' . self::escape($route->path) . '">' . self::escape($label) . '</a>';
    }
}`
}

func phpServiceTaskInputSource() string {
	return `final readonly class TaskInput
{
    public function __construct(
        public string $method,
        public string $route,
        public array $routeParameters,
        public array $query,
        public array $headers,
        public string $body,
        public mixed $payload,
    ) {}

}`
}

func phpServiceTaskResultSource() string {
	return `final readonly class TaskResult implements JsonSerializable
{
    public function __construct(
        public string $output,
        public string $error,
        public array $state,
    ) {}

    public static function success(string $output = '', array $state = []): self
    {
        return new self($output, '', $state);
    }

    public static function failure(string $error, array $state = []): self
    {
        if (trim($error) === '') {
            throw new InvalidArgumentException('TaskResult failure requires a non-empty error.');
        }
        return new self('', $error, $state);
    }

    public function jsonSerialize(): array
    {
        return ['output' => $this->output, 'error' => $this->error, 'state' => $this->state];
    }
}`
}

func phpServiceAssertionsSource() string {
	return `final class RuntimeAssertions
{
    public static function requireResult(TaskResult $result): void
    {
        if ($result->output !== '' && $result->error !== '') {
            throw new RuntimeException('TaskResult cannot contain both output and error.');
        }
    }

    public static function require(TaskResult $result, bool $condition, string $failure): void
    {
        self::requireResult($result);
        if (trim($failure) === '') {
            throw new InvalidArgumentException('Verification failure text cannot be empty.');
        }
        if (!$condition) {
            throw new RuntimeException($failure);
        }
    }
}`
}
