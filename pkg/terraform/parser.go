package terraform

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parser is an interface for parsing terraform execution result
type Parser interface {
	Parse(body string) ParseResult
}

// ParseResult represents the result of parsed terraform execution
type ParseResult struct {
	Result             string
	OutsideTerraform   string
	ChangedResult      string
	Warning            string
	HasAddOrUpdateOnly bool
	HasDestroy         bool
	HasNoChanges       bool
	HasPlanError       bool
	HasParseError      bool
	ExitCode           int
	Error              error
	CreatedResources   []string
	UpdatedResources   []string
	DeletedResources   []string
	ReplacedResources  []string
}

// DefaultParser is a parser for terraform commands
type DefaultParser struct{}

// PlanParser is a parser for terraform plan
type PlanParser struct {
	Pass          *regexp.Regexp
	Fail          *regexp.Regexp
	HasDestroy    *regexp.Regexp
	HasNoChanges  *regexp.Regexp
	Create        *regexp.Regexp
	Update        *regexp.Regexp
	Delete        *regexp.Regexp
	Replace       *regexp.Regexp
	ReplaceOption *regexp.Regexp
}

// ApplyParser is a parser for terraform apply
type ApplyParser struct {
	Pass *regexp.Regexp
	Fail *regexp.Regexp
}

// NewDefaultParser is DefaultParser initializer
func NewDefaultParser() *DefaultParser {
	return &DefaultParser{}
}

// NewPlanParser is PlanParser initialized with its Regexp
func NewPlanParser() *PlanParser {
	return &PlanParser{
		Pass: regexp.MustCompile(`(?m)^(Plan: \d|No changes.)`),
		Fail: regexp.MustCompile(`(?m)^(Error: )`),
		// "0 to destroy" should be treated as "no destroy"
		HasDestroy:    regexp.MustCompile(`(?m)([1-9][0-9]* to destroy.)`),
		HasNoChanges:  regexp.MustCompile(`(?m)^(No changes.)`),
		Create:        regexp.MustCompile(`^ *# (.*) will be created$`),
		Update:        regexp.MustCompile(`^ *# (.*) will be updated in-place$`),
		Delete:        regexp.MustCompile(`^ *# (.*) will be destroyed$`),
		Replace:       regexp.MustCompile(`^ *# (.*?)(?: is tainted, so)? must be replaced$`),
		ReplaceOption: regexp.MustCompile(`^ *# (.*?) will be replaced, as requested$`),
	}
}

// NewApplyParser is ApplyParser initialized with its Regexp
func NewApplyParser() *ApplyParser {
	return &ApplyParser{
		Pass: regexp.MustCompile(`(?m)^(Apply complete!)`),
		Fail: regexp.MustCompile(`(?m)^(Error: )`),
	}
}

// Parse returns ParseResult related with terraform commands
func (p *DefaultParser) Parse(body string) ParseResult {
	return ParseResult{
		Result:   body,
		ExitCode: ExitPass,
		Error:    nil,
	}
}

func extractAllResources(pattern *regexp.Regexp, body string) []string {
	return pattern.FindAllString(body, -1)
}

// Parse returns ParseResult related with terraform plan
func (p *PlanParser) Parse(body string) ParseResult { //noli nt:cyclop
	var exitCode int
	var hasPlanError bool
	switch {
	case p.Pass.MatchString(body):
		exitCode = ExitPass
		hasPlanError = false
	case p.Fail.MatchString(body):
		exitCode = ExitFail
		hasPlanError = true
	default:
		return ParseResult{
			Result:        "",
			HasParseError: true,
			ExitCode:      ExitFail,
			Error:         errors.New("cannot parse plan result"),
		}
	}

	lines := strings.Split(body, "\n")

	outsideBlocks := processOutsideBlocksInBody(lines)
	changeBlocks := processChangeBlocksInBody(lines)
	warningBlocks := processWarningBlocksInBody(lines)
	planResults := processPlanLinesInBody(lines)
	createdResources := extractAllResources(p.Create, body)
	updatedResources := extractAllResources(p.Update, body)
	deletedResources := extractAllResources(p.Delete, body)
	replacedResources := extractAllResources(p.Replace, body)
	replacedOptResources := extractAllResources(p.ReplaceOption, body)
	replacedResources = append(replacedResources, replacedOptResources...)

	var result string
	var hasDestroy, hasNoChanges, hasAddOrUpdateOnly bool
	if hasPlanError {
		errorBlocks := processErrorBlocksInBody(lines)
		result = strings.Join(errorBlocks, "\n")
	} else {
		result = planResults
		hasDestroy = resultHasDestroys(result)
		hasNoChanges = !resultHasChanges(result) && !resultHasAdds(result) && !resultHasDestroys(result)
		hasAddOrUpdateOnly = (resultHasChanges(result) || resultHasAdds(result)) && !resultHasDestroys(result)
	}

	outsideTerraform := ""
	if len(outsideBlocks) > 0 {
		outsideTerraform = strings.Join(outsideBlocks, "\n")
	}

	changeResult := ""
	if len(changeBlocks) > 0 {
		changeResult = strings.Join(changeBlocks, "\n")
		changeResult = fmt.Sprintf("%s\n\n%s", changeResult, result)
	}

	warnings := ""
	if len(warningBlocks) > 0 {
		warnings = strings.Join(warningBlocks, "\n")
	}

	return ParseResult{
		Result:             result,
		ChangedResult:      changeResult,
		OutsideTerraform:   outsideTerraform,
		Warning:            warnings,
		HasAddOrUpdateOnly: hasAddOrUpdateOnly,
		HasDestroy:         hasDestroy,
		HasNoChanges:       hasNoChanges,
		HasPlanError:       hasPlanError,
		ExitCode:           exitCode,
		Error:              nil,
		CreatedResources:   createdResources,
		UpdatedResources:   updatedResources,
		DeletedResources:   deletedResources,
		ReplacedResources:  replacedResources,
	}
}

func resultHasAdds(result string) bool {
	return resultHasField(result, 1)
}

func resultHasChanges(result string) bool {
	return resultHasField(result, 2)
}

func resultHasField(result string, field int) bool {
	regex := regexp.MustCompile("[0-9]+")
	values := regex.FindAllString(result, field)
	if values == nil {
		return false
	}

	changes, err := strconv.Atoi(values[field-1])
	if err != nil {
		return false
	}

	if changes <= 0 {
		return false
	}

	return true
}

func resultHasDestroys(result string) bool {
	return resultHasField(result, 3)
}

func processOutsideBlocksInBody(bodyLines []string) []string {
	return processBlocksInBody(bodyLines, 0, StartOfOutsideBlock, EndOfOutsideBlock)
}

func processChangeBlocksInBody(bodyLines []string) []string {
	return processBlocksInBody(bodyLines, 0, StartOfChangeBlock, EndOfChangeBlock)
}

func processWarningBlocksInBody(bodyLines []string) []string {
	return processBlocksInBody(bodyLines, 0, StartOfWarningBlock, EndOfWarningBlock)
}

func processErrorBlocksInBody(bodyLines []string) []string {
	return matchErrorLines(bodyLines)
}

func matchErrorLines(body []string) []string {
	var result []string
	for _, line := range body {
		if strings.Contains(line, "error") || strings.Contains(line, "Error") {
			result = append(result, line)
		}
	}

	return result
}

func processBlocksInBody(bodyLines []string, startIndex int, startPattern string, endPattern string) []string {
	workingArea := bodyLines[startIndex:]
	var results []string
	for index, line := range workingArea {
		if strings.HasPrefix(line, startPattern) {
			block, nextIndex := extractBlock(workingArea, index+1, endPattern)
			results = append(results, block)
			return append(results, processBlocksInBody(workingArea, nextIndex, startPattern, endPattern)...)
		}
	}

	return results
}

func extractBlock(body []string, start int, endPattern string) (string, int) {
	var results strings.Builder

	var nextLineAfterBlock int
	for index, line := range body[start:] {
		if strings.HasPrefix(line, endPattern) {
			nextLineAfterBlock = index + 1 + start
			break
		}

		results.WriteString(line)
		results.WriteString("\n")
	}

	result := strings.TrimRight(results.String(), "\n")
	return result, nextLineAfterBlock
}

func processPlanLinesInBody(body []string) string {
	var totalAdd, totalChange, totalDestroy int
	results := processLinesInBody(body, 0, EndOfChangeBlock)
	regex := regexp.MustCompile("[0-9]+")

	if len(results) <= 0 {
		return NoChanges
	}

	for _, plan := range results {
		values := regex.FindAllString(plan, -1)

		add, err := strconv.Atoi(values[0])
		if err != nil {
			return ""
		}
		totalAdd += add

		change, err := strconv.Atoi(values[1])
		if err != nil {
			return ""
		}
		totalChange += change

		destroy, err := strconv.Atoi(values[2])
		if err != nil {
			return ""
		}
		totalDestroy += destroy
	}

	return fmt.Sprintf("Plan: %d to add, %d to change, %d to destroy.", totalAdd, totalChange, totalDestroy)
}

func processLinesInBody(body []string, startIndex int, pattern string) []string {
	workingArea := body[startIndex:]
	var results []string
	for _, line := range workingArea {
		if strings.HasPrefix(line, pattern) {
			results = append(results, line)
		}
	}

	return results
}

// Parse returns ParseResult related with terraform apply
func (p *ApplyParser) Parse(body string) ParseResult {
	var exitCode int
	switch {
	case p.Pass.MatchString(body):
		exitCode = ExitPass
	case p.Fail.MatchString(body):
		exitCode = ExitFail
	default:
		return ParseResult{
			Result:        "",
			ExitCode:      ExitFail,
			HasParseError: true,
			Error:         errors.New("cannot parse apply result"),
		}
	}
	lines := strings.Split(body, "\n")
	var i int
	var result, line string
	for i, line = range lines {
		if p.Pass.MatchString(line) || p.Fail.MatchString(line) {
			break
		}
	}
	switch {
	case p.Pass.MatchString(line):
		result = lines[i]
	case p.Fail.MatchString(line):
		result = strings.Join(trimLastNewline(lines[i:]), "\n")
	}
	return ParseResult{
		Result:   result,
		ExitCode: exitCode,
		Error:    nil,
	}
}

func trimLastNewline(s []string) []string {
	if len(s) == 0 {
		return s
	}
	last := len(s) - 1
	if s[last] == "" {
		return s[:last]
	}
	return s
}
