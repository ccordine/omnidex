package main

import (
	"fmt"
	"strings"
)

type SolveRequest struct {
	Expression string `json:"expression"`
	Operation  string `json:"operation"`
}

type SolveResponse struct {
	Expression string   `json:"expression"`
	Operation  string   `json:"operation"`
	Result     string   `json:"result"`
	Steps      []string `json:"steps"`
}

var derivativeRules = map[string]SolveResponse{
	"x^2": {Expression: "x^2", Operation: "derivative", Result: "2x", Steps: []string{"Use the power rule d/dx x^n = n*x^(n-1).", "For n=2, d/dx x^2 = 2x."}},
	"x^3": {Expression: "x^3", Operation: "derivative", Result: "3x^2", Steps: []string{"Use the power rule.", "For n=3, d/dx x^3 = 3x^2."}},
	"sin(x)": {Expression: "sin(x)", Operation: "derivative", Result: "cos(x)", Steps: []string{"Use the standard trig derivative.", "d/dx sin(x) = cos(x)."}},
	"cos(x)": {Expression: "cos(x)", Operation: "derivative", Result: "-sin(x)", Steps: []string{"Use the standard trig derivative.", "d/dx cos(x) = -sin(x)."}},
	"e^x": {Expression: "e^x", Operation: "derivative", Result: "e^x", Steps: []string{"The natural exponential is its own derivative.", "d/dx e^x = e^x."}},
}

var integralRules = map[string]SolveResponse{
	"x": {Expression: "x", Operation: "integral", Result: "x^2/2 + C", Steps: []string{"Use the power rule for antiderivatives.", "Integral of x is x^2/2 + C."}},
	"x^2": {Expression: "x^2", Operation: "integral", Result: "x^3/3 + C", Steps: []string{"Raise the power by one.", "Divide by the new power: x^3/3 + C."}},
	"sin(x)": {Expression: "sin(x)", Operation: "integral", Result: "-cos(x) + C", Steps: []string{"Find a function whose derivative is sin(x).", "d/dx[-cos(x)] = sin(x)."}},
	"cos(x)": {Expression: "cos(x)", Operation: "integral", Result: "sin(x) + C", Steps: []string{"Find a function whose derivative is cos(x).", "d/dx sin(x) = cos(x)."}},
	"e^x": {Expression: "e^x", Operation: "integral", Result: "e^x + C", Steps: []string{"The natural exponential is its own antiderivative.", "Integral of e^x is e^x + C."}},
}

func SolveCalculus(expression, operation string) (SolveResponse, error) {
	expression = normalizeExpression(expression)
	operation = strings.ToLower(strings.TrimSpace(operation))
	var table map[string]SolveResponse
	switch operation {
	case "derivative":
		table = derivativeRules
	case "integral":
		table = integralRules
	default:
		return SolveResponse{}, fmt.Errorf("unsupported operation %q", operation)
	}
	if result, ok := table[expression]; ok {
		return result, nil
	}
	return SolveResponse{}, fmt.Errorf("unsupported expression %q", expression)
}

func normalizeExpression(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}
