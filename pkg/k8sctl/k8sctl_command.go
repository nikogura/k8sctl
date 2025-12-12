package k8sctl

import (
	"encoding/json"
	"github.com/pkg/errors"
	"os"
)

type K8sCtlCommand struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Description string   `json:"description"`
	Args        []string `json:"args"`
	Role        string   `json:"role"`
}

type K8sCtlCommands struct {
	Commands    []*K8sCtlCommand          `json:"commands"`
	CommandsMap map[string]*K8sCtlCommand `json:"-"`
}

type K8sCtlCommandResult struct {
	CommandName string `json:"command_name"`
	Command     string `json:"command"`
}

func LoadK8sCtlCommandsFromFile(filepath string) (commands *K8sCtlCommands, err error) {
	userBytes, readErr := os.ReadFile(filepath)
	if readErr != nil {
		err = errors.Wrapf(readErr, "failed loading file %s", filepath)
		return commands, err
	}

	commands, err = LoadK8sCtlCommandsFromBytes(userBytes)
	if err != nil {
		err = errors.Wrapf(err, "failed loading Users from data in %s", filepath)
	}

	return commands, err
}

func LoadK8sCtlCommandsFromBytes(userBytes []byte) (commands *K8sCtlCommands, err error) {
	commands = &K8sCtlCommands{}

	err = json.Unmarshal(userBytes, commands)
	if err != nil {
		err = errors.Wrapf(err, "failed unmarshalling CliUsers")
		return commands, err
	}

	return commands, err
}
