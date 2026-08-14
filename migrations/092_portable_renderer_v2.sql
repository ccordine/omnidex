LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE;

ALTER TABLE station_gap_openings
    DROP CONSTRAINT station_gap_openings_renderer_version_check;

ALTER TABLE station_gap_openings
    ADD CONSTRAINT station_gap_openings_renderer_version_check CHECK (
        renderer_version IN (
            'omnidex.render-portable-job.v1',
            'omnidex.render-portable-job.v2'
        )
    );
