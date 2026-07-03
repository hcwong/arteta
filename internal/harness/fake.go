package harness

import "github.com/hcwong/arteta/internal/workflow"

// Fake is a configurable Harness for tests. Zero value is safe; all methods
// return benign defaults.
type Fake struct {
	FakeID          string
	FakeDisplayName string
	FakeLaunchCmd   func(resumeID string) string
	FakeHookConfig  *HookConfig
	FakeDetectState func(content string) (workflow.State, bool)
}

func (f *Fake) ID() string { return f.FakeID }

func (f *Fake) DisplayName() string {
	if f.FakeDisplayName != "" {
		return f.FakeDisplayName
	}
	return f.FakeID
}

func (f *Fake) LaunchCommand(resumeID string) string {
	if f.FakeLaunchCmd != nil {
		return f.FakeLaunchCmd(resumeID)
	}
	if resumeID != "" {
		return f.FakeID + " --resume " + resumeID
	}
	return f.FakeID
}

func (f *Fake) HookConfig() *HookConfig { return f.FakeHookConfig }

func (f *Fake) DetectState(content string) (workflow.State, bool) {
	if f.FakeDetectState != nil {
		return f.FakeDetectState(content)
	}
	return workflow.StateUnknown, false
}
