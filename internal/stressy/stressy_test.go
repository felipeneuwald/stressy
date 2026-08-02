package stressy

import "testing"

func TestNew(t *testing.T) {
	s := New(Cfg{Workers: 4, Timeout: 30})

	if s.workers != 4 {
		t.Errorf("workers = %d, want 4", s.workers)
	}
	if s.timeout != 30 {
		t.Errorf("timeout = %d, want 30", s.timeout)
	}
	if s.done == nil {
		t.Error("done = nil, want an initialised channel")
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Cfg
		wantErr string
	}{
		{name: "one worker, indefinite", cfg: Cfg{Workers: 1, Timeout: 0}},
		{name: "one worker, one second", cfg: Cfg{Workers: 1, Timeout: 1}},
		{name: "many workers", cfg: Cfg{Workers: 64, Timeout: 60}},
		{name: "zero workers", cfg: Cfg{Workers: 0, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative workers", cfg: Cfg{Workers: -1, Timeout: 0}, wantErr: "workers must be 1 or greater"},
		{name: "negative timeout", cfg: Cfg{Workers: 1, Timeout: -1}, wantErr: "timeout must be 0 (indefinite) or greater"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.cfg).validateConfig()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateConfig() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validateConfig() error = nil, want %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("validateConfig() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestRunRejectsInvalidConfig covers Run's validation gate. It is safe to call
// Run here because validateConfig fails before any worker goroutine is started
// — a successful Run would saturate the CPU and block until signalled.
func TestRunRejectsInvalidConfig(t *testing.T) {
	if err := New(Cfg{Workers: 0}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}

	if err := New(Cfg{Workers: 1, Timeout: -1}).Run(); err == nil {
		t.Error("Run() error = nil, want a validation error")
	}
}
