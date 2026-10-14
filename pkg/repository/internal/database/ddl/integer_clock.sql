-- Integer hypertable retention and columnstore policies need a nanosecond clock
-- before their DDL runs. STABLE prevents a prepared plan freezing this value.
CREATE OR REPLACE FUNCTION public.get_unix_ns_now() RETURNS BIGINT
    LANGUAGE SQL STABLE PARALLEL SAFE AS
$$
    SELECT (EXTRACT(EPOCH FROM statement_timestamp()) * 1000000000)::BIGINT;
$$;
