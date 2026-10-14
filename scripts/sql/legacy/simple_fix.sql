-- Simple fix: Make the trigger send the actual log data as JSON
-- This is what the original system was designed for

CREATE OR REPLACE FUNCTION notify_channel()
RETURNS TRIGGER AS $$
BEGIN
    BEGIN
        -- Send the log data as JSON (original design)
        PERFORM pg_notify(TG_ARGV[0], row_to_json(NEW)::text);
    EXCEPTION
        WHEN OTHERS THEN
            RAISE WARNING 'Failed to send notification on channel %: %', TG_ARGV[0], SQLERRM;
    END;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql; 