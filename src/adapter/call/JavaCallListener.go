package call

import (
	"coca/src/adapter/models"
	"coca/src/language/java"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"reflect"
	"strings"
)

var imports []string
var clzs []string
var currentPkg string
var currentClz string
var fields []models.JAppField
var methods []models.JMethod
var methodCalls []models.JMethodCall
var currentType string

var mapFields = make(map[string]string)
var localVars = make(map[string]string)
var formalParameters = make(map[string]string)
var currentClzExtends = ""
var currentMethod models.JMethod

func NewJavaCallListener() *JavaCallListener {
	currentClz = ""
	currentPkg = ""
	currentMethod = models.NewJMethod()

	methods = nil
	methodCalls = nil
	fields = nil
	return &JavaCallListener{}
}

type JavaCallListener struct {
	parser.BaseJavaParserListener
}

func (s *JavaCallListener) getNodeInfo() *models.JClassNode {
	return &models.JClassNode{currentPkg, currentClz, currentType, "", fields, methods, methodCalls}
}

func (s *JavaCallListener) EnterPackageDeclaration(ctx *parser.PackageDeclarationContext) {
	currentPkg = ctx.QualifiedName().GetText()
}

func (s *JavaCallListener) EnterImportDeclaration(ctx *parser.ImportDeclarationContext) {
	importText := ctx.QualifiedName().GetText()
	imports = append(imports, importText)
}

func (s *JavaCallListener) EnterClassDeclaration(ctx *parser.ClassDeclarationContext) {
	currentType = "Class"
	if ctx.IDENTIFIER() != nil {
		currentClz = ctx.IDENTIFIER().GetText()
	}

	if ctx.EXTENDS() != nil {
		currentClzExtends = ctx.TypeType().GetText()
	}
}

func (s *JavaCallListener) EnterInterfaceDeclaration(ctx *parser.InterfaceDeclarationContext) {
	currentType = "Interface"
	currentClz = ctx.IDENTIFIER().GetText()
}

func (s *JavaCallListener) EnterInterfaceMethodDeclaration(ctx *parser.InterfaceMethodDeclarationContext) {
	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.IDENTIFIER().GetSymbol().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	name := ctx.IDENTIFIER().GetText()
	stopLinePosition := startLinePosition + len(name)

	typeType := ctx.TypeTypeOrVoid().GetText()

	method := &models.JMethod{name, typeType, startLine, startLinePosition, stopLine, stopLinePosition, nil, nil}
	methods = append(methods, *method)
}

func (s *JavaCallListener) EnterFormalParameter(ctx *parser.FormalParameterContext) {
	formalParameters[ctx.VariableDeclaratorId().GetText()] = ctx.TypeType().GetText()
}

func (s *JavaCallListener) EnterFieldDeclaration(ctx *parser.FieldDeclarationContext) {
	decelerators := ctx.VariableDeclarators()
	typeType := decelerators.GetParent().GetChild(0).(antlr.ParseTree).GetText()
	for _, declarator := range decelerators.(*parser.VariableDeclaratorsContext).AllVariableDeclarator() {
		value := declarator.(*parser.VariableDeclaratorContext).VariableDeclaratorId().(*parser.VariableDeclaratorIdContext).IDENTIFIER().GetText()
		mapFields[value] = typeType
		fields = append(fields, *&models.JAppField{Type: typeType, Value: value})
	}
}

func (s *JavaCallListener) EnterLocalVariableDeclaration(ctx *parser.LocalVariableDeclarationContext) {
	typ := ctx.GetChild(0).(antlr.ParseTree).GetText()
	if ctx.GetChild(1) != nil {
		if ctx.GetChild(1).GetChild(0) != nil && ctx.GetChild(1).GetChild(0).GetChild(0) != nil {

			variableName := ctx.GetChild(1).GetChild(0).GetChild(0).(antlr.ParseTree).GetText()
			localVars[variableName] = typ
		}
	}
}

func (s *JavaCallListener) EnterMethodDeclaration(ctx *parser.MethodDeclarationContext) {
	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.IDENTIFIER().GetSymbol().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	name := ctx.IDENTIFIER().GetText()
	stopLinePosition := startLinePosition + len(name)

	typeType := ctx.TypeTypeOrVoid().GetText()

	method := &models.JMethod{name, typeType, startLine, startLinePosition, stopLine, stopLinePosition, nil, nil}

	if ctx.FormalParameters() != nil {
		if ctx.FormalParameters().GetChild(0) == nil || ctx.FormalParameters().GetText() == "()" || ctx.FormalParameters().GetChild(1) == nil {
			return
		}

		var methodParams []models.JParameter = nil
		parameterList := ctx.FormalParameters().GetChild(1).(*parser.FormalParameterListContext)
		formalParameter := parameterList.AllFormalParameter()
		for _, param := range formalParameter {
			paramContext := param.(*parser.FormalParameterContext)
			paramType := paramContext.TypeType().GetText()
			paramValue := paramContext.VariableDeclaratorId().(*parser.VariableDeclaratorIdContext).IDENTIFIER().GetText()

			localVars[paramValue] = paramType
			methodParams = append(methodParams, *&models.JParameter{paramType, paramValue})
		}

		method.Parameters = methodParams
	}

	currentMethod = *method
	//methods = append(methods, *method)
}

func (s *JavaCallListener) ExitMethodDeclaration(ctx *parser.MethodDeclarationContext) {
	methods = append(methods, currentMethod)
	//fmt.Println(currentMethod)
	currentMethod = models.NewJMethod()
}

func (s *JavaCallListener) EnterCreator(ctx *parser.CreatorContext) {
	variableName := ctx.GetParent().GetParent().GetChild(0).(antlr.ParseTree).GetText()
	localVars[variableName] = ctx.CreatedName().GetText()
}

func (s *JavaCallListener) EnterLocalTypeDeclaration(ctx *parser.LocalTypeDeclarationContext) {

}

func (s *JavaCallListener) EnterMethodCall(ctx *parser.MethodCallContext) {
	var targetCtx = ctx.GetParent().GetChild(0).(antlr.ParseTree).GetText()
	var targetType = parseTargetType(targetCtx)
	callee := ctx.GetChild(0).(antlr.ParseTree).GetText()

	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.GetStart().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	stopLinePosition := startLinePosition + len(callee)

	// TODO: 处理链试调用
	if strings.Contains(targetType, "()") && strings.Contains(targetType, ".") {
		split := strings.Split(targetType, ".")
		sourceTarget := split[0]
		targetType = localVars[sourceTarget]
	}

	fullType := warpTargetFullType(targetType)
	if targetType == "super" {
		targetType = currentClzExtends
	}

	var jMethodCall = &models.JMethodCall{}
	if fullType != "" {
		jMethodCall = &models.JMethodCall{removeTarget(fullType), "", targetType, callee, startLine, startLinePosition, stopLine, stopLinePosition}
	} else {
		if ctx.GetText() == targetType {
			//fmt.Println(ctx.IDENTIFIER().GetText())
			//fmt.Println(imports)
			methodName := ctx.IDENTIFIER().GetText()
			pkg := currentPkg
			clz := currentClz
			for _, imp := range imports {
				if strings.HasSuffix(imp, "."+methodName) {
					pkg = imp
					clz = ""
				}
			}
			jMethodCall = &models.JMethodCall{pkg, "", clz, methodName, startLine, startLinePosition, stopLine, stopLinePosition}
		} else {
			methodName := ctx.IDENTIFIER().GetText()
			targetType = buildSpecificTarget(targetType)

			jMethodCall = &models.JMethodCall{currentPkg, "NEEDFIX", targetType, methodName, startLine, startLinePosition, stopLine, stopLinePosition}
		}
	}

	methodCalls = append(methodCalls, *jMethodCall)
	currentMethod.MethodCalls = append(currentMethod.MethodCalls, *jMethodCall)
}

func buildSpecificTarget(targetType string) string {
	isSelfFieldCall := strings.Contains(targetType, "this.")
	if isSelfFieldCall {
		targetType = strings.ReplaceAll(targetType, "this.", "")
		for _, field := range fields {
			if field.Value == targetType {
				targetType = field.Type
			}
		}
	}
	return targetType
}

func (s *JavaCallListener) EnterExpression(ctx *parser.ExpressionContext) {
	// lambda BlogPO::of
	if ctx.COLONCOLON() != nil {
		if ctx.Expression(0) == nil {
			return
		}

		text := ctx.Expression(0).GetText()
		methodName := ctx.IDENTIFIER().GetText()
		targetType := parseTargetType(text)
		fullType := warpTargetFullType(targetType)

		startLine := ctx.GetStart().GetLine()
		startLinePosition := ctx.GetStart().GetColumn()
		stopLine := ctx.GetStop().GetLine()
		stopLinePosition := startLinePosition + len(text)

		jMethodCall := &models.JMethodCall{removeTarget(fullType), "", targetType, methodName, startLine, startLinePosition, stopLine, stopLinePosition}
		methodCalls = append(methodCalls, *jMethodCall)
	}
}

func (s *JavaCallListener) appendClasses(classes []string) {
	clzs = classes
}

func removeTarget(fullType string) string {
	split := strings.Split(fullType, ".")
	return strings.Join(split[:len(split)-1], ".")
}

func parseTargetType(targetCtx string) string {
	targetVar := targetCtx
	targetType := targetVar

	//TODO: update this reflect
	typeOf := reflect.TypeOf(targetCtx).String()
	if strings.HasSuffix(typeOf, "MethodCallContext") {
		targetType = currentClz
	} else {
		fieldType := mapFields[targetVar]
		formalType := formalParameters[targetVar]
		localVarType := localVars[targetVar]
		if fieldType != "" {
			targetType = fieldType
		} else if formalType != "" {
			targetType = formalType
		} else if localVarType != "" {
			targetType = localVarType
		}
	}

	return targetType
}

func warpTargetFullType(targetType string) string {
	if strings.EqualFold(currentClz, targetType) {
		return currentPkg + "." + targetType
	}

	// TODO: update for array
	split := strings.Split(targetType, ".")
	str := split[0]
	pureTargetType := strings.ReplaceAll(strings.ReplaceAll(str, "[", ""), "]", "")

	for index := range imports {
		imp := imports[index]
		if strings.HasSuffix(imp, pureTargetType) {
			return imp
		}
	}

	//maybe the same package
	for _, clz := range clzs {
		if strings.HasSuffix(clz, "."+pureTargetType) {
			return clz
		}
	}

	//1. current package, 2. import by *
	if pureTargetType == "super" {
		for index := range imports {
			imp := imports[index]
			if strings.HasSuffix(imp, currentClzExtends) {
				return imp
			}
		}
	}

	return ""
}