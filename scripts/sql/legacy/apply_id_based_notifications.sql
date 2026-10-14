-- Apply ID-based notifications for better scalability and efficiency
-- This change makes the system work with minimal payload notifications
-- while maintaining compatibility with existing embedding workers

-- Update the notify_channel function to send ID-based notifications
CREATE OR REPLACE FUNCTION notify_channel()
RETURNS TRIGGER AS $$
BEGIN
    -- Send notification to the specified channel (passed as trigger argument)
    -- Use EXCEPTION block to prevent trigger failures from affecting data insertion
    BEGIN
        -- Send minimal payload with ID and timestamp for efficiency
        -- This reduces network overhead and improves scalability
        PERFORM pg_notify(TG_ARGV[0], json_build_object(
            'id', NEW.id,
            'timestamp_ns', NEW.timestamp_ns,
            'service_name', NEW.service_name
        )::text);
    EXCEPTION
        WHEN OTHERS THEN
            -- Log the error but don't fail the trigger
            RAISE WARNING 'Failed to send notification on channel %: %', TG_ARGV[0], SQLERRM;
    END;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Verify the function was updated
SELECT 
    proname as function_name,
    prosrc as function_source
FROM pg_proc 
WHERE proname = 'notify_channel'; 