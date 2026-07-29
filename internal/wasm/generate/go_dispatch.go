package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.minekube.com/gate/internal/wasm/model"
)

type OperationKind string

const (
	OperationFunction        OperationKind = "function"
	OperationMethod          OperationKind = "method"
	OperationConstantGet     OperationKind = "constant-get"
	OperationVariableGet     OperationKind = "variable-get"
	OperationVariableSet     OperationKind = "variable-set"
	OperationEventSubscribe  OperationKind = "event-subscribe"
	OperationCommandRegister OperationKind = "command-register"
	OperationTimerAfter      OperationKind = "timer-after"
	OperationTimerEvery      OperationKind = "timer-every"
	OperationContextCanceled OperationKind = "context-cancelled"
	OperationContextDeadline OperationKind = "context-deadline"
	OperationContextError    OperationKind = "context-error"
	OperationContextLog      OperationKind = "context-log"
)

const eventSubscriptionSuffix = "#wasm-subscribe"
const commandRegistrationSuffix = "#wasm-register-command"
const timerAfterSuffix = "#wasm-after"
const timerEverySuffix = "#wasm-every"
const contextCancelledSuffix = "#wasm-context-cancelled"
const contextDeadlineSuffix = "#wasm-context-deadline"
const contextErrorSuffix = "#wasm-context-error"
const contextLogSuffix = "#wasm-log"

type GeneratedOperation struct {
	ID                  uint32        `json:"id"`
	Identity            string        `json:"identity"`
	DeclarationIdentity string        `json:"declarationIdentity"`
	WITName             string        `json:"witName"`
	Kind                OperationKind `json:"kind"`
}

func Operations(api *model.API) ([]GeneratedOperation, error) {
	normalized, err := normalizedAPI(api)
	if err != nil {
		return nil, err
	}
	var operations []GeneratedOperation
	add := func(
		declaration model.Declaration,
		identity string,
		witName string,
		kind OperationKind,
	) {
		operations = append(operations, GeneratedOperation{
			Identity: identity, DeclarationIdentity: declaration.Identity,
			WITName: witName, Kind: kind,
		})
	}
	for _, declaration := range normalized.Declarations {
		if declaration.Coverage.State != model.CoverageRepresented {
			continue
		}
		switch declaration.Kind {
		case model.DeclarationFunction:
			kind := OperationFunction
			switch {
			case strings.HasSuffix(declaration.Identity, eventSubscriptionSuffix):
				kind = OperationEventSubscribe
			case strings.HasSuffix(declaration.Identity, commandRegistrationSuffix):
				kind = OperationCommandRegister
			case strings.HasSuffix(declaration.Identity, timerAfterSuffix):
				kind = OperationTimerAfter
			case strings.HasSuffix(declaration.Identity, timerEverySuffix):
				kind = OperationTimerEvery
			case strings.HasSuffix(declaration.Identity, contextCancelledSuffix):
				kind = OperationContextCanceled
			case strings.HasSuffix(declaration.Identity, contextDeadlineSuffix):
				kind = OperationContextDeadline
			case strings.HasSuffix(declaration.Identity, contextErrorSuffix):
				kind = OperationContextError
			case strings.HasSuffix(declaration.Identity, contextLogSuffix):
				kind = OperationContextLog
			}
			add(
				declaration,
				declaration.Identity,
				declaration.WITName,
				kind,
			)
		case model.DeclarationMethod:
			add(
				declaration,
				declaration.Identity,
				declaration.WITName,
				OperationMethod,
			)
		case model.DeclarationConstant:
			add(
				declaration,
				declaration.Identity+"#get",
				"get-"+declaration.WITName,
				OperationConstantGet,
			)
		case model.DeclarationVariable:
			if declaration.Variable == nil || declaration.Variable.Readable {
				add(
					declaration,
					declaration.Identity+"#get",
					"get-"+declaration.WITName,
					OperationVariableGet,
				)
			}
			if declaration.Variable != nil && declaration.Variable.Writable {
				add(
					declaration,
					declaration.Identity+"#set",
					"set-"+declaration.WITName,
					OperationVariableSet,
				)
			}
		}
	}
	slices.SortFunc(operations, func(left, right GeneratedOperation) int {
		return strings.Compare(left.Identity, right.Identity)
	})
	for index := range operations {
		operations[index].ID = uint32(index + 1)
	}
	return operations, nil
}

// RenderGoDispatch renders one statically-bound handler per Gate operation.
func RenderGoDispatch(api *model.API) ([]byte, error) {
	normalized, err := normalizedAPI(api)
	if err != nil {
		return nil, err
	}
	operations, err := Operations(normalized)
	if err != nil {
		return nil, err
	}
	witHash, err := generatedWITHash(normalized)
	if err != nil {
		return nil, err
	}
	declarations := make(
		map[string]model.Declaration,
		len(normalized.Declarations),
	)
	var packagePaths []string
	for _, declaration := range normalized.Declarations {
		declarations[declaration.Identity] = declaration
		if declaration.Coverage.State == model.CoverageRepresented {
			packagePaths = append(packagePaths, declaration.PackagePath)
			packagePaths = append(
				packagePaths,
				genericArgumentPackages(declaration.Identity)...,
			)
		}
	}
	slices.Sort(packagePaths)
	packagePaths = slices.Compact(packagePaths)
	aliases := make(map[string]string, len(packagePaths))
	for index, path := range packagePaths {
		aliases[path] = fmt.Sprintf("p%03d", index)
	}
	genericArities := genericOriginArities(normalized.Declarations)

	var output bytes.Buffer
	fmt.Fprintln(&output, "// Code generated by gate-wasm-gen; DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package api")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "import (")
	fmt.Fprintln(&output, "\t\"context\"")
	fmt.Fprintln(&output)
	fmt.Fprintln(
		&output,
		"\t\"go.minekube.com/gate/internal/wasm/runtime/dispatch\"",
	)
	for _, path := range packagePaths {
		fmt.Fprintf(
			&output,
			"\t%s %s\n",
			aliases[path],
			strconv.Quote(path),
		)
	}
	fmt.Fprintln(&output, ")")
	fmt.Fprintln(&output)
	fmt.Fprintf(
		&output,
		"const GeneratedDispatchWITHash = %s\n",
		strconv.Quote(witHash),
	)
	fmt.Fprintln(&output)
	renderOperationDescriptors(&output, operations)
	fmt.Fprintln(&output)
	renderCompileReferences(
		&output,
		normalized.Declarations,
		aliases,
		genericArities,
	)
	fmt.Fprintln(&output)
	renderOperationRegistration(&output, operations)
	for _, operation := range operations {
		fmt.Fprintln(&output)
		declaration := declarations[operation.DeclarationIdentity]
		renderOperationHandler(
			&output,
			operation,
			declaration,
			declarations,
			aliases,
			genericArities,
		)
	}
	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated Go dispatch: %w", err)
	}
	return formatted, nil
}

func renderOperationDescriptors(
	output *bytes.Buffer,
	operations []GeneratedOperation,
) {
	fmt.Fprintln(output, "type GeneratedOperationDescriptor struct {")
	fmt.Fprintln(output, "\tID uint32")
	fmt.Fprintln(output, "\tIdentity string")
	fmt.Fprintln(output, "\tDeclarationIdentity string")
	fmt.Fprintln(output, "\tWITName string")
	fmt.Fprintln(output, "\tKind string")
	fmt.Fprintln(output, "}")
	fmt.Fprintln(output)
	fmt.Fprintln(
		output,
		"var GeneratedOperations = [...]GeneratedOperationDescriptor{",
	)
	for _, operation := range operations {
		fmt.Fprintln(output, "\t{")
		fmt.Fprintf(output, "\t\tID: %d,\n", operation.ID)
		fmt.Fprintf(output, "\t\tIdentity: %s,\n", strconv.Quote(operation.Identity))
		fmt.Fprintf(
			output,
			"\t\tDeclarationIdentity: %s,\n",
			strconv.Quote(operation.DeclarationIdentity),
		)
		fmt.Fprintf(output, "\t\tWITName: %s,\n", strconv.Quote(operation.WITName))
		fmt.Fprintf(
			output,
			"\t\tKind: %s,\n",
			strconv.Quote(string(operation.Kind)),
		)
		fmt.Fprintln(output, "\t},")
	}
	fmt.Fprintln(output, "}")
}

func renderCompileReferences(
	output *bytes.Buffer,
	declarations []model.Declaration,
	aliases map[string]string,
	genericArities map[string]int,
) {
	fmt.Fprintln(
		output,
		"// Compile-time references make source API drift a Go compiler error.",
	)
	for index, declaration := range declarations {
		if declaration.Coverage.State != model.CoverageRepresented {
			continue
		}
		if strings.Contains(declaration.Identity, "#wasm-") {
			continue
		}
		alias := aliases[declaration.PackagePath]
		name := fmt.Sprintf("generatedDeclarationReference%04d", index)
		switch declaration.Kind {
		case model.DeclarationType, model.DeclarationAlias:
			expression := alias + "." + declaration.GoName
			if arity := genericArities[genericOrigin(declaration.Identity)]; arity > 0 {
				expression += genericTypeArguments(
					declaration.Identity,
					aliases,
					arity,
				)
			}
			fmt.Fprintf(
				output,
				"func %s(_ %s) {}\n",
				name,
				expression,
			)
		case model.DeclarationMethod:
			expression := methodExpression(
				declaration,
				alias,
				genericArities,
			)
			fmt.Fprintf(output, "var %s = %s\n", name, expression)
		default:
			expression := alias + "." + declaration.GoName
			if declaration.Kind == model.DeclarationFunction {
				if arity := genericArities[genericOrigin(declaration.Identity)]; arity > 0 {
					expression += genericTypeArguments(
						declaration.Identity,
						aliases,
						arity,
					)
				}
			}
			fmt.Fprintf(output, "var %s = %s\n", name, expression)
		}
	}
}

func renderOperationRegistration(
	output *bytes.Buffer,
	operations []GeneratedOperation,
) {
	fmt.Fprintln(
		output,
		"func RegisterGeneratedOperations(host *dispatch.Host) error {",
	)
	for _, operation := range operations {
		fmt.Fprintln(output, "\tif err := host.Register(dispatch.Operation{")
		fmt.Fprintf(output, "\t\tID: %d,\n", operation.ID)
		fmt.Fprintf(
			output,
			"\t\tIdentity: %s,\n",
			strconv.Quote(operation.Identity),
		)
		fmt.Fprintf(
			output,
			"\t\tHandler: generatedDispatch%04d,\n",
			operation.ID,
		)
		fmt.Fprintln(output, "\t}); err != nil {")
		fmt.Fprintln(output, "\t\treturn err")
		fmt.Fprintln(output, "\t}")
	}
	fmt.Fprintln(output, "\treturn nil")
	fmt.Fprintln(output, "}")
}

func renderOperationHandler(
	output *bytes.Buffer,
	operation GeneratedOperation,
	declaration model.Declaration,
	declarations map[string]model.Declaration,
	aliases map[string]string,
	genericArities map[string]int,
) {
	fmt.Fprintf(
		output,
		"func generatedDispatch%04d(ctx context.Context, host *dispatch.Host, arguments []any) ([]any, error) {\n",
		operation.ID,
	)
	fmt.Fprintln(output, "\toperation := dispatch.Operation{")
	fmt.Fprintf(output, "\t\tID: %d,\n", operation.ID)
	fmt.Fprintf(
		output,
		"\t\tIdentity: %s,\n",
		strconv.Quote(operation.Identity),
	)
	fmt.Fprintln(output, "\t}")
	alias := aliases[declaration.PackagePath]
	switch operation.Kind {
	case OperationFunction:
		expression := alias + "." + declaration.GoName
		if arity := genericArities[genericOrigin(declaration.Identity)]; arity > 0 {
			expression += genericTypeArguments(
				declaration.Identity,
				aliases,
				arity,
			)
		}
		fmt.Fprintf(
			output,
			"\treturn host.Call(ctx, operation, %s, arguments, %t)\n",
			expression,
			declaration.Callable != nil && declaration.Callable.Variadic,
		)
	case OperationMethod:
		if receiverUsesResource(declaration, declarations) {
			fmt.Fprintf(
				output,
				"\treturn host.CallResourceMethod(ctx, operation, %s, %s, arguments, %t)\n",
				strconv.Quote(declaration.Receiver.TypeIdentity),
				strconv.Quote(declaration.GoName),
				declaration.Callable != nil && declaration.Callable.Variadic,
			)
		} else {
			fmt.Fprintf(
				output,
				"\treturn host.Call(ctx, operation, %s, arguments, %t)\n",
				methodExpression(declaration, alias, genericArities),
				declaration.Callable != nil && declaration.Callable.Variadic,
			)
		}
	case OperationConstantGet, OperationVariableGet:
		fmt.Fprintln(output, "\t_ = operation")
		fmt.Fprintf(
			output,
			"\treturn []any{%s.%s}, nil\n",
			alias,
			declaration.GoName,
		)
	case OperationVariableSet:
		fmt.Fprintln(output, "\tif len(arguments) != 1 {")
		fmt.Fprintln(output, "\t\treturn nil, host.ArgumentCount(operation, len(arguments), 1)")
		fmt.Fprintln(output, "\t}")
		fmt.Fprintf(
			output,
			"\tif err := host.Assign(operation, &%s.%s, arguments[0]); err != nil {\n",
			alias,
			declaration.GoName,
		)
		fmt.Fprintln(output, "\t\treturn nil, err")
		fmt.Fprintln(output, "\t}")
		fmt.Fprintln(output, "\treturn nil, nil")
	case OperationEventSubscribe:
		eventIdentity := strings.TrimSuffix(
			declaration.Identity,
			eventSubscriptionSuffix,
		)
		event := declarations[eventIdentity]
		eventAlias := aliases[event.PackagePath]
		fmt.Fprintln(
			output,
			"\tsignature := func(int, func(*"+eventAlias+"."+event.GoName+
				") error) (func(), error) { return nil, nil }",
		)
		fmt.Fprintln(
			output,
			"\treturn host.CallExtension(ctx, operation, signature, arguments)",
		)
	case OperationCommandRegister:
		fmt.Fprintln(
			output,
			"\tsignature := func(string, []string, func(*"+
				aliases[declaration.PackagePath]+
				".Context) error) (func(), error) { return nil, nil }",
		)
		fmt.Fprintln(
			output,
			"\treturn host.CallExtension(ctx, operation, signature, arguments)",
		)
	case OperationTimerAfter, OperationTimerEvery:
		fmt.Fprintln(
			output,
			"\tsignature := func(int64, func() error) (func(), error) { return nil, nil }",
		)
		fmt.Fprintln(
			output,
			"\treturn host.CallExtension(ctx, operation, signature, arguments)",
		)
	case OperationContextCanceled:
		fmt.Fprintln(output, "\tsignature := func(context.Context) bool { return false }")
		fmt.Fprintln(output, "\treturn host.CallExtension(ctx, operation, signature, arguments)")
	case OperationContextDeadline:
		fmt.Fprintln(output, "\tsignature := func(context.Context) (int64, bool) { return 0, false }")
		fmt.Fprintln(output, "\treturn host.CallExtension(ctx, operation, signature, arguments)")
	case OperationContextError:
		fmt.Fprintln(output, "\tsignature := func(context.Context) string { return \"\" }")
		fmt.Fprintln(output, "\treturn host.CallExtension(ctx, operation, signature, arguments)")
	case OperationContextLog:
		fmt.Fprintln(output, "\tsignature := func(context.Context, int64, string, []string) error { return nil }")
		fmt.Fprintln(output, "\treturn host.CallExtension(ctx, operation, signature, arguments)")
	}
	fmt.Fprintln(output, "}")
}

func receiverUsesResource(
	declaration model.Declaration,
	declarations map[string]model.Declaration,
) bool {
	if declaration.Receiver == nil {
		return false
	}
	if declaration.Receiver.Pointer {
		return true
	}
	receiver, exists := declarations[declaration.Receiver.TypeIdentity]
	return !exists ||
		receiver.Type == nil ||
		receiver.Type.Kind == model.TypeResource ||
		receiver.Type.Kind == model.TypeCallback ||
		receiver.Type.Kind == model.TypeDynamic
}

func methodExpression(
	declaration model.Declaration,
	alias string,
	genericArities map[string]int,
) string {
	receiverIdentity := declaration.Receiver.TypeIdentity
	short := receiverIdentity
	if dot := strings.LastIndexByte(short, '.'); dot >= 0 {
		short = short[dot+1:]
	}
	if bracket := strings.IndexByte(short, '['); bracket >= 0 {
		short = short[:bracket]
	}
	expression := alias + "." + short
	if arity := genericArities[genericOrigin(receiverIdentity)]; arity > 0 {
		expression += anyTypeArguments(arity)
	}
	if declaration.Receiver.Pointer {
		expression = "(*" + expression + ")"
	}
	return expression + "." + declaration.GoName
}

func genericOrigin(identity string) string {
	if bracket := strings.IndexByte(identity, '['); bracket >= 0 {
		return identity[:bracket]
	}
	return identity
}

func genericOriginArities(declarations []model.Declaration) map[string]int {
	arities := make(map[string]int)
	for _, declaration := range declarations {
		identity := declaration.Identity
		open := strings.IndexByte(identity, '[')
		if open < 0 || !strings.HasSuffix(identity, "]") {
			continue
		}
		origin := identity[:open]
		arguments := identity[open+1 : len(identity)-1]
		arities[origin] = max(arities[origin], topLevelArgumentCount(arguments))
	}
	return arities
}

func topLevelArgumentCount(arguments string) int {
	if arguments == "" {
		return 0
	}
	count := 1
	depth := 0
	for _, character := range arguments {
		switch character {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
		case ',':
			if depth == 0 {
				count++
			}
		}
	}
	return count
}

func anyTypeArguments(arity int) string {
	arguments := make([]string, arity)
	for index := range arguments {
		arguments[index] = "any"
	}
	return "[" + strings.Join(arguments, ", ") + "]"
}

var qualifiedTypePattern = regexp.MustCompile(
	`[A-Za-z_][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)*\.[A-Z][A-Za-z0-9_]*`,
)

func qualifiedTypePackages(expression string) []string {
	var packages []string
	for _, match := range qualifiedTypePattern.FindAllString(expression, -1) {
		dot := strings.LastIndexByte(match, '.')
		if dot > 0 {
			packages = append(packages, match[:dot])
		}
	}
	return packages
}

func genericArgumentPackages(identity string) []string {
	open := strings.IndexByte(identity, '[')
	if open < 0 || !strings.HasSuffix(identity, "]") {
		return nil
	}
	return qualifiedTypePackages(identity[open+1 : len(identity)-1])
}

func genericTypeArguments(
	identity string,
	aliases map[string]string,
	arity int,
) string {
	open := strings.IndexByte(identity, '[')
	if open < 0 || !strings.HasSuffix(identity, "]") {
		return anyTypeArguments(arity)
	}
	arguments := identity[open+1 : len(identity)-1]
	if arguments == "" || arguments == "T" || arguments == "R" {
		return anyTypeArguments(arity)
	}
	paths := qualifiedTypePackages(arguments)
	slices.SortFunc(paths, func(left, right string) int {
		return len(right) - len(left)
	})
	for _, path := range paths {
		if alias := aliases[path]; alias != "" {
			arguments = strings.ReplaceAll(arguments, path+".", alias+".")
		}
	}
	return "[" + arguments + "]"
}
