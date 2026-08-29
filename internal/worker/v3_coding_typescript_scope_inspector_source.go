package worker

const directCodingTypeScriptScopeInspectorSource = `import path from 'node:path';
import ts from 'typescript';

const schema = 'omnidex.typescript-lexical-scope.v4';
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
const availableSymbols = new Set(symbols);
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
const deterministicRepairs = [];
const expressionIdentities = new Set();
function referencedLocalBindings(node) {
  const names = new Set();
  function collect(current) {
    if (ts.isIdentifier(current)) {
      const symbol = checker.getSymbolAtLocation(current);
      if (symbol && availableSymbols.has(symbol)) names.add(symbol.getName());
    }
    current.forEachChild(collect);
  }
  collect(node);
  return [...names].sort();
}
function containsAnyType(type, visited = new Set()) {
  if (!type || visited.has(type)) return false;
  visited.add(type);
  if ((type.flags & ts.TypeFlags.Any) !== 0) return true;
  if (typeof type.isUnionOrIntersection === 'function' && type.isUnionOrIntersection()) {
    for (const constituent of type.types) {
      if (containsAnyType(constituent, visited)) return true;
    }
  }
  for (const argument of type.aliasTypeArguments ?? []) {
    if (containsAnyType(argument, visited)) return true;
  }
  for (const argument of type.typeArguments ?? []) {
    if (containsAnyType(argument, visited)) return true;
  }
  if ((type.objectFlags & ts.ObjectFlags.Reference) !== 0 &&
      typeof checker.getTypeArguments === 'function') {
    let argumentsForReference = [];
    try {
      argumentsForReference = checker.getTypeArguments(type);
    } catch {
      argumentsForReference = [];
    }
    for (const argument of argumentsForReference) {
      if (containsAnyType(argument, visited)) return true;
    }
  }
  return false;
}
function exactTypeofPrimitive(type) {
  if (!type) return '';
  if ((type.flags & ts.TypeFlags.NumberLike) !== 0) return 'number';
  if ((type.flags & ts.TypeFlags.StringLike) !== 0) return 'string';
  if ((type.flags & ts.TypeFlags.BooleanLike) !== 0) return 'boolean';
  return '';
}
function isExactlyNullishType(type) {
  if (!type) return false;
  const constituents = type.isUnion() ? type.types : [type];
  return constituents.length > 0 && constituents.every((constituent) =>
    (constituent.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined)) !== 0,
  );
}
function exactContextualTypeofPrimitive(type, visited = new Set()) {
  const direct = exactTypeofPrimitive(type);
  if (direct || !type || !type.isUnion()) return direct;
  if (visited.has(type)) return '';
  visited.add(type);
  let primitive = '';
  let hasDirectPrimitive = false;
  for (const constituent of type.types) {
    if (isExactlyNullishType(constituent)) continue;
    const constituentPrimitive = exactTypeofPrimitive(constituent);
    if (constituentPrimitive) {
      if (primitive && primitive !== constituentPrimitive) return '';
      primitive = constituentPrimitive;
      hasDirectPrimitive = true;
      continue;
    }
    const signatures = checker.getSignaturesOfType(constituent, ts.SignatureKind.Call);
    if (signatures.length === 0) return '';
    for (const signature of signatures) {
      if (signature.parameters.length !== 0) return '';
      const returnedPrimitive = exactContextualTypeofPrimitive(
        checker.getReturnTypeOfSignature(signature), new Set(visited),
      );
      if (!returnedPrimitive || (primitive && primitive !== returnedPrimitive)) return '';
      primitive = returnedPrimitive;
    }
  }
  return hasDirectPrimitive ? primitive : '';
}
function isStableReferenceExpression(node) {
  if (ts.isIdentifier(node)) return true;
  if (ts.isParenthesizedExpression(node)) return isStableReferenceExpression(node.expression);
  if (ts.isPropertyAccessExpression(node)) return isStableReferenceExpression(node.expression);
  if (ts.isElementAccessExpression(node)) {
    return isStableReferenceExpression(node.expression) &&
      (ts.isStringLiteral(node.argumentExpression) || ts.isNumericLiteral(node.argumentExpression));
  }
  return false;
}
function sameStableReferenceExpression(left, right) {
  if (ts.isParenthesizedExpression(left)) return sameStableReferenceExpression(left.expression, right);
  if (ts.isParenthesizedExpression(right)) return sameStableReferenceExpression(left, right.expression);
  if (ts.isIdentifier(left) && ts.isIdentifier(right)) {
    const leftSymbol = checker.getSymbolAtLocation(left);
    const rightSymbol = checker.getSymbolAtLocation(right);
    return Boolean(leftSymbol) && leftSymbol === rightSymbol;
  }
  if (ts.isPropertyAccessExpression(left) && ts.isPropertyAccessExpression(right)) {
    return left.name.text === right.name.text &&
      sameStableReferenceExpression(left.expression, right.expression);
  }
  if (ts.isElementAccessExpression(left) && ts.isElementAccessExpression(right)) {
    const leftArgument = left.argumentExpression;
    const rightArgument = right.argumentExpression;
    const sameArgument = ts.isStringLiteral(leftArgument) && ts.isStringLiteral(rightArgument)
      ? leftArgument.text === rightArgument.text
      : ts.isNumericLiteral(leftArgument) && ts.isNumericLiteral(rightArgument) &&
        leftArgument.text === rightArgument.text;
    return sameArgument && sameStableReferenceExpression(left.expression, right.expression);
  }
  return false;
}
function hasExactPrimitiveUnionMismatch(inferred, contextual, primitive) {
  const constituents = inferred.isUnion() ? inferred.types : [inferred];
  let hasCompatible = false;
  let hasIncompatibleNonNullish = false;
  for (const constituent of constituents) {
    if (checker.isTypeAssignableTo(constituent, contextual)) {
      const constituentPrimitive = exactTypeofPrimitive(constituent);
      if (constituentPrimitive === primitive) {
        hasCompatible = true;
      } else if (!isExactlyNullishType(constituent)) {
        return false;
      }
      continue;
    }
    if ((constituent.flags & (ts.TypeFlags.Null | ts.TypeFlags.Undefined)) === 0) {
      hasIncompatibleNonNullish = true;
    }
  }
  return hasCompatible && hasIncompatibleNonNullish;
}
function exactPrimitiveLiteral(node, primitive) {
  if (primitive === 'string') return ts.isStringLiteral(node);
  if (primitive === 'number') return ts.isNumericLiteral(node);
  if (primitive === 'boolean') {
    return node.kind === ts.SyntaxKind.TrueKeyword || node.kind === ts.SyntaxKind.FalseKeyword;
  }
  return false;
}
function exactNullLiteral(node) {
  return node.kind === ts.SyntaxKind.NullKeyword;
}
function priorPrimitiveNarrowing(reference, contextual, before) {
  const narrowings = new Map();
  function collect(node) {
    const start = node.getStart(sourceFile);
    const end = node.getEnd();
    if (start >= blockStart && end <= blockEnd && end <= before && ts.isConditionalExpression(node)) {
      const condition = node.condition;
      if (ts.isBinaryExpression(condition) &&
          condition.operatorToken.kind === ts.SyntaxKind.EqualsEqualsEqualsToken &&
          ts.isTypeOfExpression(condition.left) && ts.isStringLiteral(condition.right) &&
          sameStableReferenceExpression(condition.left.expression, reference) &&
          sameStableReferenceExpression(node.whenTrue, reference) &&
          (exactPrimitiveLiteral(node.whenFalse, condition.right.text) ||
            exactNullLiteral(node.whenFalse))) {
        const primitive = condition.right.text;
        const narrowedType = checker.getTypeAtLocation(node);
        if (exactContextualTypeofPrimitive(narrowedType) === primitive &&
            checker.isTypeAssignableTo(narrowedType, contextual)) {
          const source = node.getText(sourceFile).trim();
          if (!/[\r\n]/.test(source) && source.length <= 2048) {
            const startByte = Buffer.byteLength(sourceFile.text.slice(blockStart, start), 'utf8');
            narrowings.set(source + '\u0000' + startByte, { primitive, source, startByte });
          }
        }
      }
    }
    node.forEachChild(collect);
  }
  sourceFile.forEachChild(collect);
  return narrowings.size === 1 ? [...narrowings.values()][0] : null;
}
function deterministicPrimitiveNullishRepair(
  candidate, evidenceIndex, inferred, contextual, incompatibleTypes,
) {
  const node = candidate.node;
  if (!contextual || incompatibleTypes.length === 0 || !ts.isBinaryExpression(node) ||
      node.operatorToken.kind !== ts.SyntaxKind.QuestionQuestionToken ||
      !isStableReferenceExpression(node.left)) return null;
  const primitive = exactContextualTypeofPrimitive(contextual);
  if (!primitive) return null;
  const fallbackType = checker.getTypeAtLocation(node.right);
  if (!checker.isTypeAssignableTo(fallbackType, contextual) ||
      (exactTypeofPrimitive(fallbackType) !== primitive &&
        !(exactNullLiteral(node.right) && isExactlyNullishType(fallbackType)))) return null;
  const leftType = checker.getTypeAtLocation(node.left);
  if (!hasExactPrimitiveUnionMismatch(leftType, contextual, primitive)) return null;
  const left = node.left.getText(sourceFile).trim();
  const fallback = node.right.getText(sourceFile).trim();
  const replacement = 'typeof ' + left + " === '" + primitive + "' ? " + left + ' : ' + fallback;
  const startByte = Buffer.byteLength(sourceFile.text.slice(blockStart, candidate.start), 'utf8');
  return {
    mechanism: 'deterministic_primitive_nullish_narrowing',
    evidence_index: evidenceIndex,
    source: candidate.source,
    replacement,
    start_byte: startByte,
    end_byte: Buffer.byteLength(sourceFile.text.slice(blockStart, candidate.end), 'utf8'),
    normalization_start_byte: startByte,
  };
}
function deterministicPrimitiveReferenceRepair(
  candidate, evidenceIndex, inferred, contextual, incompatibleTypes,
) {
  const node = candidate.node;
  if (!contextual || incompatibleTypes.length === 0 || !isStableReferenceExpression(node)) return null;
  const narrowing = priorPrimitiveNarrowing(node, contextual, candidate.start);
  if (!narrowing ||
      !hasExactPrimitiveUnionMismatch(inferred, contextual, narrowing.primitive)) return null;
  return {
    mechanism: 'deterministic_primitive_reference_narrowing',
    evidence_index: evidenceIndex,
    source: candidate.source,
    replacement: narrowing.source,
    start_byte: Buffer.byteLength(sourceFile.text.slice(blockStart, candidate.start), 'utf8'),
    end_byte: Buffer.byteLength(sourceFile.text.slice(blockStart, candidate.end), 'utf8'),
    normalization_start_byte: narrowing.startByte,
  };
}
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
  if (contextual && !containsAnyType(contextual) &&
      typeof checker.isTypeAssignableTo === 'function') {
    const constituents = inferred.isUnion() ? inferred.types : [inferred];
    for (const constituent of constituents) {
      if (!checker.isTypeAssignableTo(constituent, contextual)) {
        incompatibleTypes.push(checker.typeToString(constituent, candidate.node, typeFlags));
      }
    }
    incompatibleTypes.sort();
  }
  const referencedBindings = referencedLocalBindings(candidate.node);
  const evidenceIndex = expressionEvidence.length;
  const deterministicRepair = deterministicPrimitiveNullishRepair(
    candidate, evidenceIndex, inferred, contextual, incompatibleTypes,
  ) ?? deterministicPrimitiveReferenceRepair(
    candidate, evidenceIndex, inferred, contextual, incompatibleTypes,
  );
  if (deterministicRepair) deterministicRepairs.push(deterministicRepair);
  expressionEvidence.push({
    source: candidate.source,
    inferred_type: inferredType,
    ...(contextualType ? { contextual_type: contextualType } : {}),
    ...(incompatibleTypes.length > 0 ? { incompatible_types: [...new Set(incompatibleTypes)] } : {}),
    ...(referencedBindings.length > 0 ? { referenced_bindings: referencedBindings } : {}),
  });
}
process.stdout.write(JSON.stringify({
  schema,
  bindings,
  unavailable_bindings: unavailableBindings,
  expression_evidence: expressionEvidence,
  deterministic_repairs: deterministicRepairs,
}));
`
