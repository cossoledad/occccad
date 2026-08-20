package jobs

import "testing"

func TestPopulateCapabilities(t *testing.T) {
	tests := []struct {
		name                string
		job                 Job
		canCancel, canRetry bool
	}{
		{"queued", Job{State: "QUEUED"}, true, false},
		{"running export", Job{Type: "EXCHANGE_EXPORT", State: "RUNNING", Progress: 95}, true, false},
		{"import candidate phase", Job{Type: "EXCHANGE_IMPORT", State: "RUNNING", Progress: 69}, true, false},
		{"import commit phase", Job{Type: "EXCHANGE_IMPORT", State: "RUNNING", Progress: 70}, false, false},
		{"cancel requested", Job{State: "RUNNING", CancelRequestedAt: stringPointer("now")}, false, false},
		{"failed", Job{State: "FAILED"}, false, true},
		{"canceled", Job{State: "CANCELED"}, false, true},
		{"succeeded", Job{State: "SUCCEEDED"}, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			populateCapabilities(&test.job)
			if test.job.CanCancel != test.canCancel || test.job.CanRetry != test.canRetry {
				t.Fatalf("capabilities = cancel %v, retry %v", test.job.CanCancel, test.job.CanRetry)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }
