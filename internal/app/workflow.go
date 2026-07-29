package app

import (
	"context"

	"github.com/xy200303/MobBase/internal/system"
)

// executeWorkflowCommand preserves the terminal behavior of official mobile
// tools while keeping stdout machine-safe for JSON callers. Event-mode callers
// receive each output line as a structured log event.
func (r runtime) executeWorkflowCommand(ctx context.Context, command, program string, args, environment []string, directory string) (system.CommandResult, error) {
	if r.json && r.eventMode {
		err := system.StreamLines(ctx, program, args, environment, directory, func(line string) error {
			return r.emit("log", command, true, map[string]string{"stream": "stdout", "output": line}, nil)
		}, func(line string) error {
			return r.emit("log", command, true, map[string]string{"stream": "stderr", "output": line}, nil)
		})
		return system.CommandResult{}, err
	}
	if !r.json {
		err := system.Stream(ctx, program, args, environment, directory, r.out, r.err)
		return system.CommandResult{}, err
	}
	return system.Run(ctx, program, args, environment, directory, "")
}
