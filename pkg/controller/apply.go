package controller

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/mattn/go-colorable"
	"github.com/suzuki-shunsuke/go-timeout/timeout"
	"github.com/suzuki-shunsuke/tfcmt/pkg/apperr"
	"github.com/suzuki-shunsuke/tfcmt/pkg/notifier"
	"github.com/suzuki-shunsuke/tfcmt/pkg/platform"
)

// Apply sends the notification with notifier
func (ctrl *Controller) Apply(ctx context.Context, command Command) error {
	if err := platform.Complement(&ctrl.Config); err != nil {
		return err
	}

	if err := ctrl.Config.Validate(); err != nil {
		return err
	}

	ntf, err := ctrl.getNotifier(ctx)
	if err != nil {
		return err
	}

	if ntf == nil {
		return errors.New("no notifier specified at all")
	}

	cmd := exec.Command(command.Cmd, command.Args...) //nolint:gosec
	cmd.Stdin = os.Stdin
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	combinedOutput := &bytes.Buffer{}
	uncolorizedStdout := colorable.NewNonColorable(stdout)
	uncolorizedStderr := colorable.NewNonColorable(stderr)
	uncolorizedCombinedOutput := colorable.NewNonColorable(combinedOutput)
	cmd.Stdout = io.MultiWriter(os.Stdout, uncolorizedStdout, uncolorizedCombinedOutput)
	cmd.Stderr = io.MultiWriter(os.Stderr, uncolorizedStderr, uncolorizedCombinedOutput)
	_ = timeout.NewRunner(0).Run(ctx, cmd)

	return apperr.NewExitError(ntf.Apply(ctx, &notifier.ParamExec{
		Stdout:         stdout.String(),
		Stderr:         stderr.String(),
		CombinedOutput: combinedOutput.String(),
		CIName:         ctrl.Config.CI.Name,
		ExitCode:       cmd.ProcessState.ExitCode(),
	}))
}
