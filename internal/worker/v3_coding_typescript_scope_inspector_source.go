package worker

const directCodingTypeScriptScopeInspectorSource = `import path from 'node:path';
import ts from 'typescript';

const schema = 'omnidex.typescript-lexical-scope.v1';
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
process.stdout.write(JSON.stringify({ schema, bindings, unavailable_bindings: unavailableBindings }));
`
