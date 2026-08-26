package worker

func phpServiceHTTPLifecycleRuntime() string {
	return `function verificationLifecycleRequest(
    string $requestMedia,
    string $responseMedia,
    array $state,
    array $sentinels,
): array {
    if ($sentinels === []) {
        throw new RuntimeException('HTTP lifecycle state has no strongly observable sentinel value.');
    }
    switch ($requestMedia) {
        case 'application/json':
            $body = json_encode($state, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
            break;
        case 'application/x-www-form-urlencoded':
            $body = http_build_query($state, '', '&', PHP_QUERY_RFC3986);
            break;
        default:
            throw new LogicException('HTTP lifecycle request media is not type-preserving.');
    }
    return [
        'headers' => ['accept' => $responseMedia, 'content-type' => $requestMedia],
        'body' => $body,
        'sentinels' => $sentinels,
    ];
}

function verifyLifecycleSentinel(array $response, string $media, array $sentinels): void
{
    if ($sentinels === []) {
        throw new RuntimeException('HTTP lifecycle verification has no sentinel authority.');
    }
    $decoded = $media === 'application/json'
        ? json_decode($response['body'], true, 512, JSON_THROW_ON_ERROR)
        : null;
    $missing = [];
    foreach ($sentinels as $sentinel) {
        $observed = $media === 'application/json'
            ? verificationContainsExactValue($decoded, $sentinel)
            : str_contains($response['body'], (string) $sentinel);
        if (!$observed) {
            $missing[] = $sentinel;
        }
    }
    if ($missing !== []) {
        throw new RuntimeException(
            'A directly related read endpoint dropped one or more sentinels persisted by the write endpoint.'
        );
    }
}

function verificationContainsExactValue(mixed $value, mixed $expected): bool
{
    if ($value === $expected) {
        return true;
    }
    if (!is_array($value)) {
        return false;
    }
    foreach ($value as $nested) {
        if (verificationContainsExactValue($nested, $expected)) {
            return true;
        }
    }
    return false;
}`
}
