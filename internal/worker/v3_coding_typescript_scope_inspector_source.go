package worker

const directCodingTypeScriptScopeInspectorSource = `import path from 'node:path';
import ts from 'typescript';

const schema = 'omnidex.typescript-lexical-scope.v2';
const [relativePath, lineRaw, columnRaw, blockStartRaw, blockEndRaw] = process.argv.slice(2);
const line = Number(lineRaw);
const column = Number(columnRaw);
const blockStartLine = Number(blockStartRaw);
const blockEndLine = Number(blockEndRaw);
if (!relativePath || path.isAbsolute(relativePath) || relativePath.split(/[\\/]/).includes('..') ||
    !Number.isInteger(line) || line < 1 || !Number.isInteger(column) || column < 1 ||
    !Number.isInteger(blockStartLine) || blockStartLine < 1 ||
    !Number.isInteger(blockEndLine) || blockEndLine < blockStartLine ||
    line < blockStartLine || line > blockEndLine) {
  throw new Error('TypeScript scope inspector received invalid exact authority');
}

const configPath = ts.findConfigFile(process.cwd(), ts.sys.fileExists, 'tsconfig.json');
if (!configPath) throw new Error('TypeScript scope inspector found no tsconfig.json');
const configFile = ts.readConfigFile(configPath, ts.sys.readFile);
if (configFile.error) throw new Error(ts.flattenDiagnosticMessageText(configFile.error.messageText, '\n'));
const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, path.dirname(configPath));
if (parsed.errors.length > 0) {
  throw new Error(parsed.errors.map((error) => ts.flattenDiagnosticMessageText(error.messageText, '\n')).join('\n'));
}
const program = ts.createProgram({ rootNames: parsed.fileNames, options: parsed.options });
const checker = program.getTypeChecker();
const targetPath = path.resolve(process.cwd(), relativePath);
const sourceFile = program.getSourceFiles().find((source) => path.resolve(source.fileName) === targetPath);
if (!sourceFile) throw new Error('TypeScript scope inspector found no exact source file');
if (line > sourceFile.getLineAndCharacterOfPosition(sourceFile.end).line + 1) {
  throw new Error('TypeScript scope inspector line is outside the source file');
}
const position = sourceFile.getPositionOfLineAndCharacter(line - 1, column - 1);
const blockStart = sourceFile.getPositionOfLineAndCharacter(blockStartLine - 1, 0);
const blockEndStart = sourceFile.getPositionOfLineAndCharacter(blockEndLine - 1, 0);
const blockEnd = sourceFile.getLineEndOfPosition(blockEndStart);

function smallestContaining(node) {
  let selected = node;
  node.forEachChild((child) => {
    if (child.getFullStart() <= position && position < child.getEnd()) selected = smallestContaining(child);
  });
  return selected;
}

function declarationInsideBlock(declaration) {
  return declaration.getSourceFile() === sourceFile &&
    declaration.getStart(sourceFile) >= blockStart && declaration.getEnd() <= blockEnd;
}

const typeFlags = ts.TypeFormatFlags.NoTruncation |
  ts.TypeFormatFlags.UseAliasDefinedOutsideCurrentScope |
  ts.TypeFormatFlags.WriteArrowStyleSignature;
const identifierPattern = /^[$_\p{ID_Start}][$\u200c\u200d\p{ID_Continue}]*$/u;
const location = smallestContaining(sourceFile);
const symbols = checker.getSymbolsInScope(location, ts.SymbolFlags.Value)
  .filter((symbol) => Array.isArray(symbol.declarations) && symbol.declarations.some(declarationInsideBlock));
function renderBinding(symbol, typeLocation) {
  const name = symbol.getName();
  if (!identifierPattern.test(name)) return null;
  const type = checker.getTypeOfSymbolAtLocation(symbol, typeLocation);
  const callableSignatures = [...new Set(type.getCallSignatures()
    .map((signature) => checker.signatureToString(signature, typeLocation, typeFlags)))]
    .sort();
  const members = [...new Set(checker.getPropertiesOfType(type)
    .filter((member) => Array.isArray(member.declarations) &&
      member.declarations.some((declaration) => declaration.getSourceFile() === sourceFile))
    .map((member) => {
      const memberType = checker.getTypeOfSymbolAtLocation(member, typeLocation);
      return member.getName() + ': ' + checker.typeToString(memberType, typeLocation, typeFlags);
    })
    .filter((member) => identifierPattern.test(member.slice(0, member.indexOf(':')))))]
    .sort();
  return {
    name,
    type: checker.typeToString(type, typeLocation, typeFlags),
    ...(callableSignatures.length > 0 ? { callable_signatures: callableSignatures } : {}),
    ...(members.length > 0 ? { members } : {}),
  };
}

const bindings = [];
for (const symbol of symbols) {
  const binding = renderBinding(symbol, location);
  if (binding) bindings.push(binding);
}
bindings.sort((left, right) => left.name < right.name ? -1 : left.name > right.name ? 1 : 0);
const availableNames = new Set(bindings.map((binding) => binding.name));
const unavailableByName = new Map();
function collectNestedDeclarations(node) {
  if (node.getStart(sourceFile) < blockStart || node.getEnd() > blockEnd) return;
  if ((ts.isVariableDeclaration(node) || ts.isParameter(node) || ts.isFunctionDeclaration(node) ||
       ts.isFunctionExpression(node) || ts.isArrowFunction(node)) && node.name && ts.isIdentifier(node.name)) {
    const symbol = checker.getSymbolAtLocation(node.name);
    const name = node.name.text;
    if (symbol && identifierPattern.test(name) && !availableNames.has(name)) {
      const binding = renderBinding(symbol, node.name);
      if (binding) unavailableByName.set(name, binding);
    }
  }
  node.forEachChild(collectNestedDeclarations);
}
sourceFile.forEachChild(collectNestedDeclarations);
const unavailableBindings = [...unavailableByName.values()]
  .sort((left, right) => left.name < right.name ? -1 : left.name > right.name ? 1 : 0);

function isExpressionCandidate(node) {
  return ts.isIdentifier(node) || ts.isPropertyAccessExpression(node) ||
    ts.isElementAccessExpression(node) || ts.isCallExpression(node) ||
    ts.isNewExpression(node) || ts.isParenthesizedExpression(node) ||
    ts.isBinaryExpression(node) || ts.isConditionalExpression(node) ||
    ts.isAsExpression(node) || ts.isTypeAssertionExpression(node) ||
    ts.isNonNullExpression(node) || ts.isTemplateExpression(node) ||
    ts.isNoSubstitutionTemplateLiteral(node) || ts.isStringLiteral(node) ||
    ts.isNumericLiteral(node) || ts.isArrayLiteralExpression(node) ||
    ts.isObjectLiteralExpression(node) || node.kind === ts.SyntaxKind.TrueKeyword ||
    node.kind === ts.SyntaxKind.FalseKeyword || node.kind === ts.SyntaxKind.NullKeyword;
}

const expressionCandidates = [];
function collectExpressionCandidates(node) {
  const start = node.getStart(sourceFile);
  const end = node.getEnd();
  if (start >= blockStart && end <= blockEnd && isExpressionCandidate(node)) {
    const startLine = sourceFile.getLineAndCharacterOfPosition(start).line + 1;
    const endLine = sourceFile.getLineAndCharacterOfPosition(Math.max(start, end - 1)).line + 1;
    const source = node.getText(sourceFile).trim();
    if (startLine === line && endLine === line && source && !/[\r\n]/.test(source) && source.length <= 2048) {
      expressionCandidates.push({ node, start, end, source });
    }
  }
  node.forEachChild(collectExpressionCandidates);
}
sourceFile.forEachChild(collectExpressionCandidates);
expressionCandidates.sort((left, right) => {
  const leftContains = left.start <= position && position < left.end ? 0 : 1;
  const rightContains = right.start <= position && position < right.end ? 0 : 1;
  if (leftContains !== rightContains) return leftContains - rightContains;
  const span = (right.end - right.start) - (left.end - left.start);
  if (span !== 0) return span;
  return left.start - right.start;
});
const expressionEvidence = [];
const expressionIdentities = new Set();
for (const candidate of expressionCandidates) {
  if (expressionEvidence.length === 8) break;
  const inferred = checker.getTypeAtLocation(candidate.node);
  const inferredType = checker.typeToString(inferred, candidate.node, typeFlags);
  let contextual;
  try {
    contextual = checker.getContextualType(candidate.node);
  } catch {
    contextual = undefined;
  }
  const contextualType = contextual
    ? checker.typeToString(contextual, candidate.node, typeFlags)
    : '';
  const identity = candidate.source + '\u0000' + inferredType + '\u0000' + contextualType;
  if (!inferredType || expressionIdentities.has(identity)) continue;
  expressionIdentities.add(identity);
  const incompatibleTypes = [];
  if (contextual && typeof checker.isTypeAssignableTo === 'function') {
    const constituents = inferred.isUnion() ? inferred.types : [inferred];
    for (const constituent of constituents) {
      if (!checker.isTypeAssignableTo(constituent, contextual)) {
        incompatibleTypes.push(checker.typeToString(constituent, candidate.node, typeFlags));
      }
    }
    incompatibleTypes.sort();
  }
  expressionEvidence.push({
    source: candidate.source,
    inferred_type: inferredType,
    ...(contextualType ? { contextual_type: contextualType } : {}),
    ...(incompatibleTypes.length > 0 ? { incompatible_types: [...new Set(incompatibleTypes)] } : {}),
  });
}
process.stdout.write(JSON.stringify({
  schema,
  bindings,
  unavailable_bindings: unavailableBindings,
  expression_evidence: expressionEvidence,
}));
`
