package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nightingale-develop/reconcile-guard/internal/result"
)

type outputFormat string

const (
	outputText outputFormat = "text"
	outputJSON outputFormat = "json"
)

func parseOutputOption(
	args []string,
) ([]string, outputFormat, bool, error) {
	format := outputText
	specified := false

	clean := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		var value string
		found := false

		switch {
		case arg == "--output":
			found = true

			if i+1 >= len(args) {
				return nil, "", false, fmt.Errorf(
					"--output requires text or json",
				)
			}

			i++
			value = args[i]

		case strings.HasPrefix(arg, "--output="):
			found = true
			value = strings.TrimPrefix(
				arg,
				"--output=",
			)
		}

		if !found {
			clean = append(clean, arg)
			continue
		}

		if specified {
			return nil, "", false, fmt.Errorf(
				"--output may only be specified once",
			)
		}

		specified = true

		switch value {
		case string(outputText):
			format = outputText

		case string(outputJSON):
			format = outputJSON

		default:
			return nil, "", false, fmt.Errorf(
				"unsupported output format %q",
				value,
			)
		}
	}

	return clean, format, specified, nil
}

func isVerificationCommand(command string) bool {
	return strings.HasPrefix(command, "verify-")
}

func (c cli) writeJSON(
	command string,
	report result.Report,
) error {
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(
		result.NewDocument(command, report),
	); err != nil {
		return fmt.Errorf(
			"encode JSON output: %w",
			err,
		)
	}

	return nil
}

func (c cli) writeSingleContractJSON(
	command string,
	operatorName string,
	contract result.Contract,
) error {
	return c.writeJSON(
		command,
		result.SingleOperator(
			operatorName,
			contract,
		),
	)
}
