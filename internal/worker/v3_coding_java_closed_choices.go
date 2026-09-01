package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type javaMethodInvocationDefect struct {
	target     *treesitter.Node
	question   string
	candidates []directCodingIdentifierCandidate
	cause      error
}

func directCodingJavaTokenChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	candidates []directCodingIdentifierCandidate,
) ([]assemblyline.OpaqueModelChoice, error) {
	if node == nil {
		return nil, fmt.Errorf("Java closed choice requires one exact source token")
	}
	bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
	startByte := int(node.StartByte()) - bodyStart
	endByte := int(node.EndByte()) - bodyStart
	if startByte < 0 || endByte <= startByte || endByte > len(body) {
		return nil, fmt.Errorf("Java closed choice token is outside the implementation body")
	}
	candidates = directCodingTrialIdentifierCandidates(
		body, startByte, endByte, candidates,
		func(trial string) error {
			_, err := validateDirectCodingJavaFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices(
		"Java", string(source[node.StartByte():node.EndByte()]), candidates,
	)
}

func directCodingJavaTypeChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
) ([]assemblyline.OpaqueModelChoice, error) {
	types := javaTaskNeutralAuthorities()
	for owner := range receiverMethods {
		types[owner] = struct{}{}
	}
	candidates := make([]directCodingIdentifierCandidate, 0, len(types))
	for name := range types {
		if _, forbidden := javaForbiddenAuthority[name]; forbidden {
			continue
		}
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted type",
		})
	}
	return directCodingJavaTokenChoices(input, body, node, source, candidates)
}

func javaMethodInvocationCorrection(
	node *treesitter.Node,
	root *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	methods map[javaMethodKey]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) *javaMethodInvocationDefect {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return &javaMethodInvocationDefect{
			cause: fmt.Errorf("Java fragment has an unnamed method invocation"),
		}
	}
	name := javaNodeText(nameNode, source)
	key := javaMethodKey{Name: name, Arity: javaMethodInvocationArity(node)}
	object := node.ChildByFieldName("object")
	if _, forbidden := javaForbiddenMethods[name]; forbidden {
		owner := ""
		staticReceiver := false
		if object != nil {
			owner, staticReceiver = javaExpressionOwner(
				object, source, authorities, receiverMethods, bindings,
			)
		}
		return &javaMethodInvocationDefect{
			target: nameNode, question: "Which available method has the required meaning here?",
			candidates: javaMethodNameCandidates(
				owner, staticReceiver, object == nil, key.Arity, methods, receiverMethods,
			),
			cause: fmt.Errorf("Java fragment calls forbidden method %s", name),
		}
	}
	if object == nil {
		if _, allowed := methods[key]; allowed {
			return nil
		}
		return &javaMethodInvocationDefect{
			target: nameNode, question: "Which available direct method has the required meaning here?",
			candidates: javaMethodNameCandidates("", false, true, key.Arity, methods, receiverMethods),
			cause:      fmt.Errorf("Java fragment calls undeclared direct method %s", name),
		}
	}
	owner, staticReceiver := javaExpressionOwner(
		object, source, authorities, receiverMethods, bindings,
	)
	if owner == "" {
		if !javaAtomicReceiverNode(object) {
			return &javaMethodInvocationDefect{
				cause: fmt.Errorf(
					"Java fragment cannot isolate one exact receiver token for method %s",
					name,
				),
			}
		}
		return &javaMethodInvocationDefect{
			target: object, question: "Which available receiver provides this method?",
			candidates: javaReceiverCandidates(root, object, source, key, authorities, receiverMethods, bindings),
			cause:      fmt.Errorf("Java fragment cannot prove the owner of method %s", name),
		}
	}
	if _, forbidden := javaForbiddenAuthority[owner]; forbidden {
		if !javaAtomicReceiverNode(object) {
			return &javaMethodInvocationDefect{
				cause: fmt.Errorf(
					"Java fragment cannot isolate one exact receiver token for forbidden authority %s",
					owner,
				),
			}
		}
		return &javaMethodInvocationDefect{
			target: object, question: "Which available receiver provides this method?",
			candidates: javaReceiverCandidates(root, object, source, key, authorities, receiverMethods, bindings),
			cause:      fmt.Errorf("Java fragment uses forbidden authority %s", owner),
		}
	}
	method, allowed := javaLookupMethodAuthority(owner, key, receiverMethods)
	if allowed && method.Static == staticReceiver {
		return nil
	}
	return &javaMethodInvocationDefect{
		target: nameNode, question: "Which available method has the required meaning here?",
		candidates: javaMethodNameCandidates(
			owner, staticReceiver, false, key.Arity, methods, receiverMethods,
		),
		cause: fmt.Errorf(
			"Java fragment calls undeclared method %s/%d on %s", name, key.Arity, owner,
		),
	}
}

func javaMethodNameCandidates(
	owner string,
	staticReceiver bool,
	direct bool,
	arity int,
	methods map[javaMethodKey]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0)
	if direct {
		for key := range methods {
			if key.Arity == arity && !javaMethodForbidden(key.Name) {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: key.Name, role: "permitted direct method",
				})
			}
		}
		return candidates
	}
	appendMethods := func(values map[javaMethodKey]javaMethodAuthority) {
		for key, method := range values {
			if key.Arity == arity && method.Static == staticReceiver && !javaMethodForbidden(key.Name) {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: key.Name, role: "permitted receiver method",
				})
			}
		}
	}
	appendMethods(receiverMethods[owner])
	appendMethods(javaPureMethods[owner])
	return candidates
}

func javaReceiverCandidates(
	root *treesitter.Node,
	at *treesitter.Node,
	source []byte,
	key javaMethodKey,
	authorities map[string]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) []directCodingIdentifierCandidate {
	candidates := make([]directCodingIdentifierCandidate, 0)
	var inspect func(*treesitter.Node)
	inspect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		if node.Kind() == "identifier" {
			name := javaNodeText(node, source)
			owner, bound := bindings[name]
			method, allowed := javaLookupMethodAuthority(owner, key, receiverMethods)
			if bound && allowed && !method.Static &&
				directCodingTreeBindingAvailableAt(node, at, directCodingJavaScopeKind) {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: name, role: "in-scope receiver value",
				})
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			inspect(node.NamedChild(index))
		}
	}
	inspect(root)
	for owner := range authorities {
		method, allowed := javaLookupMethodAuthority(owner, key, receiverMethods)
		if allowed && method.Static && !javaAuthorityForbidden(owner) {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: owner, role: "permitted static receiver type",
			})
		}
	}
	return candidates
}

func javaMethodForbidden(name string) bool {
	_, forbidden := javaForbiddenMethods[name]
	return forbidden
}

func javaAuthorityForbidden(name string) bool {
	_, forbidden := javaForbiddenAuthority[name]
	return forbidden
}

func javaAtomicReceiverNode(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	return node.Kind() == "identifier" || node.Kind() == "type_identifier"
}
