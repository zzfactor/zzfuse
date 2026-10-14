-- Fix the notify_channel function to send actual row data
CREATE OR REPLACE FUNCTION notify_channel()
RETURNS TRIGGER AS $$
BEGIN
    -- Send notification to the specified channel (passed as trigger argument)
    -- Use EXCEPTION block to prevent trigger failures from affecting data insertion
    BEGIN
        -- Send the row data as JSON instead of empty string
        PERFORM pg_notify(TG_ARGV[0], row_to_json(NEW)::text);
    EXCEPTION
        WHEN OTHERS THEN
            -- Log the error but don't fail the trigger
            RAISE WARNING 'Failed to send notification on channel %: %', TG_ARGV[0], SQLERRM;
    END;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql; 