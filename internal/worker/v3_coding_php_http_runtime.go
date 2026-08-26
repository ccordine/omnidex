package worker

func phpServiceHTTPRuntimeSource() string {
	return `final readonly class HttpRequest
{
    public function __construct(
        public string $method,
        public string $path,
        public array $query,
        public array $headers,
        public array $post,
        public array $files,
        public string $body,
    ) {}
}

final readonly class HttpResponse
{
    public function __construct(
        public int $status,
        public string $media,
        public string $body,
    ) {}
}

final class HttpFailure extends RuntimeException
{
    public function __construct(public readonly int $status, string $message)
    {
        parent::__construct($message);
    }
}

final class RuntimeHttp
{
    public static function isStaticFileRequest(array $server, string $publicDirectory): bool
    {
        if (!array_key_exists('REQUEST_URI', $server) || !is_string($server['REQUEST_URI'])) {
            throw new HttpFailure(400, 'Request URI is missing.');
        }
        $path = parse_url($server['REQUEST_URI'], PHP_URL_PATH);
        if (!is_string($path) || $path === '' || $path[0] !== '/') {
            throw new HttpFailure(400, 'Request URI does not contain an absolute path.');
        }
        $decoded = rawurldecode($path);
        if (str_contains($decoded, "\0") || str_contains($decoded, '\\')) {
            throw new HttpFailure(400, 'Static asset path contains invalid bytes.');
        }
        $relative = trim($decoded, '/');
        if ($decoded !== '/' && ('/' . $relative !== $decoded || str_contains($relative, '//'))) {
            throw new HttpFailure(400, 'Static asset path is not normalized.');
        }
        foreach (explode('/', $relative) as $segment) {
            if ($segment === '.' || $segment === '..') {
                throw new HttpFailure(400, 'Static asset path cannot traverse directories.');
            }
        }
        $root = realpath($publicDirectory);
        if (!is_string($root)) {
            throw new LogicException('Public asset directory is unavailable.');
        }
        $candidate = realpath($root . $decoded);
        if (!is_string($candidate)) {
            return false;
        }
        if ($candidate === $root) {
            return false;
        }
        if (!str_starts_with($candidate, $root . DIRECTORY_SEPARATOR)) {
            throw new HttpFailure(400, 'Static asset resolved outside the public directory.');
        }
        if (!is_file($candidate)) {
            return false;
        }
        if (strtolower(pathinfo($candidate, PATHINFO_EXTENSION)) === 'php') {
            return false;
        }
        return true;
    }

    public static function fromGlobals(
        array $server,
        array $query,
        array $post,
        array $files,
        string $body,
    ): HttpRequest {
        foreach (['REQUEST_METHOD', 'REQUEST_URI'] as $required) {
            if (!array_key_exists($required, $server) || !is_string($server[$required])) {
                throw new HttpFailure(400, 'Required HTTP request metadata is missing.');
            }
        }
        $path = parse_url($server['REQUEST_URI'], PHP_URL_PATH);
        if (!is_string($path) || $path === '' || $path[0] !== '/') {
            throw new HttpFailure(400, 'Request URI does not contain an absolute path.');
        }
        $headers = [];
        foreach ($server as $key => $value) {
            if (!is_string($key) || !is_string($value)) {
                continue;
            }
            if (str_starts_with($key, 'HTTP_')) {
                $name = strtolower(str_replace('_', '-', substr($key, 5)));
                $headers[$name] = $value;
            }
        }
        if (array_key_exists('CONTENT_TYPE', $server) && is_string($server['CONTENT_TYPE'])) {
            $headers['content-type'] = $server['CONTENT_TYPE'];
        }
        $method = strtoupper($server['REQUEST_METHOD']);
        if ($method === 'POST' && array_key_exists('_method', $post)) {
            if (!is_string($post['_method'])) {
                throw new HttpFailure(400, 'HTTP method override is invalid.');
            }
            $override = strtoupper($post['_method']);
            if (!in_array($override, ['PUT', 'PATCH', 'DELETE'], true)) {
                throw new HttpFailure(400, 'HTTP method override is unsupported.');
            }
            $method = $override;
            unset($post['_method']);
        }
        return new HttpRequest($method, $path, $query, $headers, $post, $files, $body);
    }

    public static function matchRoute(string $template, string $path): ?array
    {
        if ($template === '/' || $path === '/') {
            return $template === $path ? [] : null;
        }
        $expected = explode('/', trim($template, '/'));
        $actual = explode('/', trim($path, '/'));
        if (count($expected) !== count($actual)) {
            return null;
        }
        $parameters = [];
        foreach ($expected as $index => $segment) {
            $value = $actual[$index];
            if (str_starts_with($segment, '{') && str_ends_with($segment, '}')) {
                if ($value === '') {
                    return null;
                }
                $parameters[substr($segment, 1, -1)] = rawurldecode($value);
                continue;
            }
            if ($segment !== $value) {
                return null;
            }
        }
        return $parameters;
    }

    public static function assertExposure(string $exposure): void
    {
        if ($exposure === 'public') {
            return;
        }
        if ($exposure === 'authenticated') {
            throw new LogicException('Authenticated exposure requires an identity provider and is unavailable.');
        }
        if ($exposure === 'internal') {
            throw new LogicException('Internal exposure requires a trusted network source and is unavailable.');
        }
        throw new LogicException('Endpoint exposure is not registered.');
    }

    public static function taskInput(
        HttpRequest $request,
        array $routeParameters,
        string $requestMedia,
    ): TaskInput {
        $contentType = strtolower(trim(explode(';', $request->headers['content-type'] ?? '')[0]));
        if ($requestMedia !== 'none' && $contentType !== $requestMedia) {
            throw new HttpFailure(415, 'Request media type does not match the endpoint contract.');
        }
        $payload = null;
        switch ($requestMedia) {
            case 'none':
                break;
            case 'application/json':
                try {
                    $payload = json_decode($request->body, true, 512, JSON_THROW_ON_ERROR);
                } catch (JsonException $failure) {
                    throw new HttpFailure(400, 'Request body is not valid JSON.');
                }
                break;
            case 'application/x-www-form-urlencoded':
                parse_str($request->body, $payload);
                break;
            case 'multipart/form-data':
                $payload = ['fields' => $request->post, 'files' => self::uploadedFiles($request->files)];
                break;
            case 'application/xml':
            case 'text/plain':
                $payload = $request->body;
                break;
            default:
                throw new LogicException('Request media type is not registered.');
        }
        return new TaskInput(
            $request->method, $request->path, $routeParameters, $request->query,
            $request->headers, $request->body, $payload,
        );
    }

    private static function uploadedFiles(array $files): array
    {
        $normalized = [];
        foreach ($files as $field => $file) {
            if (!is_string($field) || !is_array($file)) {
                throw new HttpFailure(400, 'Multipart file metadata is invalid.');
            }
            foreach (['name', 'type', 'tmp_name', 'error', 'size'] as $required) {
                if (!array_key_exists($required, $file) || is_array($file[$required])) {
                    throw new HttpFailure(400, 'Nested multipart files are not supported.');
                }
            }
            if ((int) $file['error'] !== UPLOAD_ERR_OK) {
                throw new HttpFailure(400, 'Multipart upload did not complete successfully.');
            }
            $content = file_get_contents((string) $file['tmp_name']);
            if (!is_string($content)) {
                throw new HttpFailure(400, 'Multipart upload content could not be read.');
            }
            $normalized[$field] = [
                'name' => basename((string) $file['name']),
                'media' => (string) $file['type'],
                'size' => (int) $file['size'],
                'content' => $content,
            ];
        }
        return $normalized;
    }

    public static function response(
        TaskResult $result,
        string $media,
        int $status,
        ?string $html = null,
    ): HttpResponse
    {
        RuntimeAssertions::requireResult($result);
        if ($result->error !== '') {
            return new HttpResponse(422, 'text/plain', $result->error);
        }
        if ($status === 204) {
            if ($html !== null) {
                throw new LogicException('A no-content response cannot carry HTML.');
            }
            return new HttpResponse(204, 'none', '');
        }
        if ($media === 'text/html') {
            if ($html === null || trim($html) === '') {
                throw new LogicException('HTML response requires one rendered representation.');
            }
            return new HttpResponse($status, $media, $html);
        }
        if ($html !== null) {
            throw new LogicException('A non-HTML response cannot carry an HTML representation.');
        }
        return new HttpResponse($status, $media, self::render($result, $media));
    }

    private static function render(TaskResult $result, string $media): string
    {
        switch ($media) {
            case 'application/json':
                return json_encode($result, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
            case 'application/xml':
                return '<result><output>' . self::escape($result->output) . '</output><state>' .
                    self::escape(json_encode($result->state, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES)) .
                    '</state></result>';
            case 'text/plain':
            case 'application/octet-stream':
                return $result->output;
            default:
                throw new LogicException('Response media type is not registered.');
        }
    }

    private static function escape(string $value): string
    {
        return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
    }

    public static function emit(HttpResponse $response): void
    {
        http_response_code($response->status);
        if ($response->media !== 'none') {
            header('Content-Type: ' . $response->media);
        }
        echo $response->body;
    }

    public static function failure(Throwable $failure): HttpResponse
    {
        if ($failure instanceof HttpFailure) {
            return new HttpResponse($failure->status, 'text/plain', $failure->getMessage());
        }
        error_log((string) $failure);
        return new HttpResponse(500, 'text/plain', 'The server could not complete the request.');
    }
}`
}
