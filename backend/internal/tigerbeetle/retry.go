package tigerbeetle

import "time"

func retry(
	maxAttempts int,
	delay time.Duration,
	operation func() error,
) error {
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = operation()

		if err == nil {
			return nil
		}

		if attempt < maxAttempts {
			time.Sleep(delay)
		}
	}

	return err
}
