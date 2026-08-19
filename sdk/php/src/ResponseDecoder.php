<?php

declare(strict_types=1);

namespace Omnidex\Integration;

final class ResponseDecoder
{
    private function __construct()
    {
    }

    /** @return array<string, mixed> */
    public static function json(string $raw): array
    {
        try {
            $value = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $error) {
            throw new \UnexpectedValueException('Omnidex returned invalid JSON.', 0, $error);
        }
        if (!is_array($value) || !str_starts_with(ltrim($raw), '{')) {
            throw new \UnexpectedValueException('Omnidex response must be one JSON object.');
        }
        return $value;
    }

    /** @param array<string, mixed> $value */
    public static function dataSource(array $value): DataSource
    {
        self::exact($value, ['source'], ['source'], 'data-source response');
        $source = self::object($value['source'] ?? null, 'data source');
        self::exact($source, self::dataSourceKeys(), ['id', 'name', 'driver', 'execution_mode', 'read_only'], 'data source');
        if (($source['driver'] ?? null) !== 'postgres' || ($source['read_only'] ?? null) !== true ||
            !in_array($source['execution_mode'] ?? null, ['direct', 'delegated'], true)) {
            throw new \UnexpectedValueException('Omnidex returned an invalid data-source authority.');
        }
        return new DataSource(
            self::string($source, 'id'),
            self::string($source, 'name'),
            'postgres',
            self::string($source, 'execution_mode'),
            true,
            $source,
        );
    }

    /** @param array<string, mixed> $value */
    public static function channelEnvelope(array $value, string $id, ?string $sourceId): Channel
    {
        self::exact($value, ['channel'], ['channel'], 'channel response');
        $channel = self::channel(self::object($value['channel'], 'channel'));
        if ($channel->id !== $id || ($sourceId !== null && $channel->dataSourceId !== $sourceId) ||
            $channel->mode !== 'assistant') {
            throw new \UnexpectedValueException('Omnidex returned a channel outside the requested authority.');
        }
        return $channel;
    }

    /** @param array<string, mixed> $value */
    public static function messageEnvelope(array $value, string $channelId, string $prompt): SendMessageResult
    {
        self::exact($value, ['channel', 'user_message', 'job'], ['channel', 'user_message', 'job'], 'message response');
        $channel = self::channel(self::object($value['channel'], 'channel'));
        $message = self::message(self::object($value['user_message'], 'user message'));
        $job = self::job(self::object($value['job'], 'job'));
        if ($channel->id !== $channelId || $message->channelId !== $channelId ||
            $message->content !== $prompt || $job->id < 1) {
            throw new \UnexpectedValueException('Omnidex returned a message outside the requested authority.');
        }
        return new SendMessageResult($channel, $message, $job);
    }

    /** @param array<string, mixed> $value */
    public static function messagePage(array $value, string $channelId): MessagePage
    {
        self::exact($value, ['channel_id', 'messages', 'next_before_id', 'has_more'],
            ['channel_id', 'messages', 'has_more'], 'message page');
        if (($value['channel_id'] ?? null) !== $channelId || !is_array($value['messages'] ?? null) ||
            !is_bool($value['has_more'] ?? null)) {
            throw new \UnexpectedValueException('Omnidex returned invalid message-page authority.');
        }
        $next = $value['next_before_id'] ?? null;
        if (($next !== null && (!is_int($next) || $next < 1)) || $value['has_more'] !== ($next !== null)) {
            throw new \UnexpectedValueException('Omnidex returned contradictory message pagination.');
        }
        $messages = array_map(
            fn (mixed $item): ChannelMessage => self::message(self::object($item, 'channel message')),
            $value['messages'],
        );
        return new MessagePage($channelId, $messages, $next, $value['has_more']);
    }

    /** @param array<string, mixed> $value */
    public static function jobDetails(array $value, int $jobId): JobDetails
    {
        self::exact($value, ['job', 'steps', 'contexts'], ['job', 'steps', 'contexts'], 'job details');
        $job = self::job(self::object($value['job'], 'job'));
        if ($job->id !== $jobId || !is_array($value['steps']) || !is_array($value['contexts'])) {
            throw new \UnexpectedValueException('Omnidex returned a different job authority.');
        }
        foreach ($value['steps'] as $step) {
            self::exact(self::object($step, 'job step'), self::stepKeys(), ['id', 'job_id', 'action', 'status'], 'job step');
        }
        foreach ($value['contexts'] as $context) {
            self::exact(self::object($context, 'job context'), ['id', 'step_id', 'key', 'value', 'created_at'],
                ['id', 'step_id', 'key', 'value', 'created_at'], 'job context');
        }
        return new JobDetails($job, $value['steps'], $value['contexts']);
    }

    /** @param array<string, mixed> $value */
    public static function error(array $value, int $status): OmnidexApiException
    {
        try {
            self::exact($value, ['error'], ['error'], 'error envelope');
            $message = self::string($value, 'error');
        } catch (\Throwable) {
            $message = 'invalid error envelope';
        }
        return new OmnidexApiException($status, $message);
    }

    /** @param array<string, mixed> $value */
    private static function channel(array $value): Channel
    {
        self::exact($value, self::channelKeys(), ['id', 'scope', 'mode'], 'channel');
        return new Channel(
            self::string($value, 'id'), (string) ($value['scope'] ?? ''), (string) ($value['name'] ?? ''),
            is_array($value['tags'] ?? null) ? $value['tags'] : [], (int) ($value['project_id'] ?? 0),
            (string) ($value['workspace_root'] ?? ''), (string) ($value['data_source_id'] ?? ''),
            self::string($value, 'mode'),
        );
    }

    /** @param array<string, mixed> $value */
    private static function message(array $value): ChannelMessage
    {
        self::exact($value, ['id', 'channel_id', 'role', 'content', 'created_at'],
            ['id', 'channel_id', 'role', 'content'], 'channel message');
        $id = $value['id'] ?? null;
        if (!is_int($id) || $id < 1 || !in_array($value['role'] ?? null, ['user', 'assistant'], true)) {
            throw new \UnexpectedValueException('Omnidex returned an invalid channel message.');
        }
        return new ChannelMessage($id, self::string($value, 'channel_id'), self::string($value, 'role'),
            self::string($value, 'content', false), (string) ($value['created_at'] ?? ''));
    }

    /** @param array<string, mixed> $value */
    private static function job(array $value): Job
    {
        self::exact($value, self::jobKeys(), ['id', 'instruction', 'pipeline'], 'job');
        $id = $value['id'] ?? null;
        if (!is_int($id) || $id < 1) {
            throw new \UnexpectedValueException('Omnidex returned an invalid job identity.');
        }
        return new Job($id, self::string($value, 'instruction', false), self::string($value, 'pipeline'),
            (string) ($value['status'] ?? ''), (string) ($value['result'] ?? ''),
            (string) ($value['error'] ?? ''), (int) ($value['current_generation'] ?? 0));
    }

    /** @param array<string, mixed> $value @param list<string> $allowed @param list<string> $required */
    private static function exact(array $value, array $allowed, array $required, string $label): void
    {
        $unknown = array_diff(array_keys($value), $allowed);
        $missing = array_diff($required, array_keys($value));
        if ($unknown !== [] || $missing !== []) {
            throw new \UnexpectedValueException($label . ' has unknown or missing fields.');
        }
    }

    /** @return array<string, mixed> */
    private static function object(mixed $value, string $label): array
    {
        if (!is_array($value)) {
            throw new \UnexpectedValueException($label . ' must be an object.');
        }
        return $value;
    }

    /** @param array<string, mixed> $value */
    private static function string(array $value, string $key, bool $nonblank = true): string
    {
        $resolved = $value[$key] ?? null;
        if (!is_string($resolved) || ($nonblank && trim($resolved) === '')) {
            throw new \UnexpectedValueException($key . ' must be a string.');
        }
        return $resolved;
    }

    /** @return list<string> */
    private static function dataSourceKeys(): array
    {
        return ['id', 'name', 'driver', 'execution_mode', 'host', 'port', 'database_name', 'username',
            'ssl_mode', 'use_dsn', 'authority_url', 'credential_env', 'read_only', 'password_set',
            'password_hint', 'last_test_status', 'last_test_message', 'last_test_at',
            'catalog_updated_at', 'created_at', 'updated_at'];
    }

    /** @return list<string> */
    private static function channelKeys(): array
    {
        return ['id', 'scope', 'name', 'tags', 'project_id', 'workspace_root', 'data_source_id', 'mode',
            'roleplay_viewpoint_character_id', 'created_at', 'updated_at'];
    }

    /** @return list<string> */
    private static function jobKeys(): array
    {
        return ['id', 'instruction', 'pipeline', 'status', 'result', 'error', 'metadata',
            'current_generation', 'created_at', 'updated_at', 'completed_at'];
    }

    /** @return list<string> */
    private static function stepKeys(): array
    {
        return ['id', 'job_id', 'action', 'sort_index', 'status', 'generation', 'superseded_at_generation',
            'worker_id', 'output', 'error', 'started_at', 'finished_at', 'created_at', 'updated_at'];
    }
}
