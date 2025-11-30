package config

type VariableExpander interface {
	Expand(input string, lookup func(string) (string, bool)) (string, error)
}
