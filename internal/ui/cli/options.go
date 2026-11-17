package cli

import (
	"fmt"
	"strings"
)

type options struct {
	management, offline                  bool
	responseFormat, responseSchema       string
	eventType, tool, operationID         string
	after, limit                         string
	continueFrom                         string
	thinking                             string
	workspace, userConfig, projectConfig string
	values                               map[string]string
	args                                 []string
	help, version                        bool
	approve, deny                        string
	attachments                          []string
}

func parse(args []string) (options, error) {
	opts := options{values: make(map[string]string)}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts.args = append(opts.args, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			opts.args = append(opts.args, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--help", "-h":
			if hasValue {
				return opts, fmt.Errorf("help does not accept a value")
			}
			opts.help = true
		case "--version":
			if hasValue {
				return opts, fmt.Errorf("version does not accept a value")
			}
			opts.version = true
		case "--no-motion", "--accessible":
			key := "ui.no_motion"
			if name == "--accessible" {
				key = "ui.accessible"
			}
			if !hasValue {
				value = "true"
			}
			opts.values[key] = value
		case "--workspace", "--user-config", "--project-config", "--provider", "--model", "--policy", "--format", "--store", "--set", "--base-url", "--approve", "--deny", "--attach", "--thinking", "--continue", "--response-format", "--response-schema", "--event-type", "--tool", "--operation-id", "--after", "--limit":
			if !hasValue {
				i++
				if i == len(args) || strings.HasPrefix(args[i], "--") {
					return opts, fmt.Errorf("%s requires a value", name)
				}
				value = args[i]
			}
			switch name {
			case "--response-format":
				opts.responseFormat = value
			case "--response-schema":
				opts.responseSchema = value
			case "--event-type":
				opts.eventType = value
			case "--tool":
				opts.tool = value
			case "--operation-id":
				opts.operationID = value
			case "--after":
				opts.after = value
			case "--limit":
				opts.limit = value
			case "--continue":
				opts.continueFrom = value
			case "--thinking":
				opts.thinking = value
				opts.values["thinking"] = value
			case "--attach":
				opts.attachments = append(opts.attachments, value)
			case "--approve":
				opts.approve = value
			case "--deny":
				opts.deny = value
			case "--base-url":
				opts.values["base_url"] = value
			case "--workspace":
				if value == "" {
					return opts, fmt.Errorf("workspace path must not be empty")
				}
				opts.workspace = value
			case "--user-config":
				if value == "" {
					return opts, fmt.Errorf("user configuration path must not be empty")
				}
				opts.userConfig = value
			case "--project-config":
				if value == "" {
					return opts, fmt.Errorf("project configuration path must not be empty")
				}
				opts.projectConfig = value
			case "--set":
				key, v, ok := strings.Cut(value, "=")
				if !ok || key == "" {
					return opts, fmt.Errorf("--set requires key=value; see config schema")
				}
				opts.values[key] = v
			default:
				key := strings.TrimPrefix(name, "--")
				if key == "format" {
					key = "output.format"
				}
				if key == "store" {
					key = "credentials.store"
				}
				opts.values[key] = value
			}
		default:
			return opts, fmt.Errorf("unknown flag; see --help")
		}
	}
	return opts, nil
}
