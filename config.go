package onlineconf

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	markerEOF     = "#EOF"
	markerSpecial = "#!"
	markerSymlink = "#@"
	jsonSuffix    = ":JSON"
)

var EOF = errors.New("EOF")

type Config struct {
	Name    string
	Version string

	Data map[string]interface{}
}

func readConfig(filename string) (*Config, error) {
	r, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	conf := new(Config)
	if err := parseConfig(r, conf); err != EOF {
		return nil, fmt.Errorf("failed to parse config: %s %v", filename, err)
	}
	return conf, nil
}

func parseConfig(r io.Reader, v *Config) (retErr error) {
	var name, version string
	data := make(map[string]interface{})

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		line = strings.TrimSpace(line)

		if len(line) == 0 {
			continue
		}
		if line == markerEOF {
			retErr = EOF
			break
		}

		if line[0] == '#' {
			if strings.HasPrefix(line, markerSpecial) {
				//fmt.Printf("found special: %s\n", line)
				if err := parseSpecial(line, &name, &version); err != nil {
					retErr = err
					return
				}
				continue
			} else if strings.HasPrefix(line, markerSymlink) {
				//fmt.Printf("found symlink: %s\n", line)
				continue
			} else {
				//fmt.Printf("found comment: %s\n", line)
				continue
			}
		}

		k, v, err := parseVar(line)
		if err != nil {
			retErr = err
			return
		}
		data[k] = v
	}

	if name == "" && version == "" {
		retErr = errors.New("\"Version\" or/and \"Name\" variables were not found")
		return
	}

	v.Name = name
	v.Version = version
	v.Data = data

	return
}

func parseSpecial(line string, name, version *string) error {
	line = strings.TrimSpace(strings.TrimPrefix(line, markerSpecial))
	key, value, err := parseLine(line)
	if err != nil {
		return err
	}
	switch strings.ToLower(key) {
	case "name":
		*name = value
	case "version":
		*version = value
	default:
		fmt.Printf("unexpected special key: %s line: %s\n", key, line)
	}
	return nil
}

func parseVar(line string) (key string, value interface{}, err error) {
	var v string
	key, v, err = parseLine(line)
	if err != nil {
		return
	}

	if strings.HasSuffix(key, jsonSuffix) {
		key = strings.TrimSuffix(key, jsonSuffix)
		jsonMap := make(map[string]interface{})
		err = json.Unmarshal([]byte(v), &jsonMap)
		if err != nil {
			err = fmt.Errorf("failed to parse json variable: %s %v", v, err)
			return
		}
		value = jsonMap
	} else {
		value = v
	}
	return
}

func parseLine(line string) (key string, value string, err error) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) > 2 {
		err = fmt.Errorf("unexpected line: %s", line)
		return
	}
	key = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	return
}
