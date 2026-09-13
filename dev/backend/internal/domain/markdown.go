package domain

import (
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ApplySource は本文全体を検証し、明示的に指定されたメタデータだけを変更する。
func ApplySource(k Knowledge, source string) (Knowledge, string, error) {
	if len(source) > MaxSourceBytes {
		return k, "", ErrTooLarge
	}

	if !utf8.ValidString(source) {
		return k, "", validation("invalid_utf8", 1, 1)
	}

	if strings.TrimSpace(source) == "" {
		return k, "", validation("empty_source", 1, 1)
	}

	if k.Format == "html" {
		return k, source, nil
	}

	if k.Format != "markdown" {
		return k, "", ErrValidation
	}

	body := source

	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return k, body, nil
	}

	lines := strings.Split(normalized, "\n")
	end := -1

	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}

	if end < 0 {
		return k, "", validation("front_matter_syntax", 1, 1)
	}

	front := strings.Join(lines[1:end], "\n")
	if len(front) > 16*1024 {
		return k, "", validation("front_matter_too_large", 1, 1)
	}

	body = strings.Join(lines[end+1:], "\n")
	if strings.TrimSpace(body) == "" {
		return k, "", validation("empty_source", end+2, 1)
	}

	var document yaml.Node

	decoder := yaml.NewDecoder(strings.NewReader(front))

	decodeErr := decoder.Decode(&document)
	if decodeErr == nil {
		var trailing yaml.Node

		decodeErr = decoder.Decode(&trailing)
		if decodeErr == nil {
			return k, "", validation("front_matter_syntax", trailing.Line+1, 1)
		}

		if errors.Is(decodeErr, io.EOF) {
			decodeErr = nil
		}
	}

	if decodeErr != nil {
		line := 2

		if match := yamlErrorLine.FindStringSubmatch(decodeErr.Error()); len(match) == 2 {
			if value, parseErr := strconv.Atoi(match[1]); parseErr == nil {
				line = value + 1
			}
		}

		return k, "", validation("front_matter_syntax", line, 1)
	}

	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return k, "", validation("front_matter_syntax", 2, 1)
	}

	node := document.Content[0]
	if bad := forbiddenYAML(node); bad != nil {
		return k, "", validation("front_matter_syntax", bad.Line+1, bad.Column)
	}

	seen := map[string]bool{}

	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if seen[key.Value] {
			return k, "", validation("front_matter_syntax", key.Line+1, key.Column)
		}

		seen[key.Value] = true
		fail := func() (Knowledge, string, error) {
			return k, "", validation("front_matter_syntax", value.Line+1, value.Column)
		}

		switch key.Value {
		case "title":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fail()
			}

			title := strings.TrimSpace(value.Value)
			if n := utf8.RuneCountInString(title); n < 1 || n > 200 {
				return fail()
			}

			k.Title = title
		case "learningStatus":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" ||
				(value.Value != "unlearned" && value.Value != "learning" && value.Value != "learned") {
				return fail()
			}

			k.LearningStatus = value.Value
		case "tags":
			if value.Kind != yaml.SequenceNode || len(value.Content) > 20 {
				return fail()
			}

			tags := []string{}
			unique := map[string]bool{}

			for _, tag := range value.Content {
				if tag.Kind != yaml.ScalarNode || tag.Tag != "!!str" {
					return fail()
				}

				text := strings.TrimSpace(tag.Value)
				if utf8.RuneCountInString(text) > 50 {
					return fail()
				}

				if text != "" && !unique[text] {
					tags = append(tags, text)
					unique[text] = true
				}
			}

			k.Tags = tags
		default:
			return k, "", validation("front_matter_unknown_key", key.Line+1, key.Column)
		}
	}

	return k, body, nil
}

func forbiddenYAML(n *yaml.Node) *yaml.Node {
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return n
	}

	for _, child := range n.Content {
		if bad := forbiddenYAML(child); bad != nil {
			return bad
		}
	}

	return nil
}

func validation(reason string, line, column int) error {
	return &ValidationError{Detail: FieldError{Field: "source", Reason: reason, Line: line, Column: column}}
}

var yamlErrorLine = regexp.MustCompile(`line ([0-9]+):`)
