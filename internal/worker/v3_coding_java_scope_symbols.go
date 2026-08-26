package worker

import "unicode"

type javaMethodKey struct {
	Name  string
	Arity int
}

type javaMethodAuthority struct {
	ReturnOwner string
	Static      bool
}

var javaForbiddenAuthority = map[string]struct{}{
	"System": {}, "ProcessBuilder": {}, "Files": {}, "File": {},
	"Path": {}, "Paths": {}, "Socket": {}, "ServerSocket": {},
	"URL": {}, "URI": {}, "URLConnection": {}, "Class": {},
	"ClassLoader": {}, "Method": {}, "Field": {}, "Constructor": {},
	"AccessibleObject": {}, "Proxy": {}, "Unsafe": {}, "MethodHandles": {},
	"RuntimePermission": {}, "Thread": {}, "Executor": {}, "Executors": {},
	"sun": {}, "jdk": {},
}

var javaForbiddenMethods = map[string]struct{}{
	"getRuntime": {}, "forName": {}, "getClass": {}, "getMethod": {},
	"getDeclaredMethod": {}, "getField": {}, "getDeclaredField": {},
	"invoke": {}, "setAccessible": {}, "load": {}, "loadLibrary": {},
	"getClassLoader": {}, "getProtectionDomain": {}, "getCodeSource": {},
	"getLocation": {}, "getResource": {}, "getResources": {},
	"getResourceAsStream": {}, "getSystemClassLoader": {},
}

func javaPureMethodAuthorities() map[string]map[javaMethodKey]javaMethodAuthority {
	table := make(map[string]map[javaMethodKey]javaMethodAuthority)
	add := func(owner, name, result string, static bool, arities ...int) {
		methods := table[owner]
		if methods == nil {
			methods = make(map[javaMethodKey]javaMethodAuthority)
			table[owner] = methods
		}
		for _, arity := range arities {
			methods[javaMethodKey{Name: name, Arity: arity}] = javaMethodAuthority{
				ReturnOwner: result, Static: static,
			}
		}
	}
	add("Map", "of", "Map", true, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20)
	add("Map", "copyOf", "Map", true, 1)
	add("Map", "get", "Object", false, 1)
	add("Map", "equals", "Boolean", false, 1)
	add("Map", "containsKey", "Boolean", false, 1)
	add("Map", "isEmpty", "Boolean", false, 0)
	add("Map", "size", "Integer", false, 0)
	add("List", "of", "List", true, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	add("List", "copyOf", "List", true, 1)
	add("List", "get", "Object", false, 1)
	add("List", "equals", "Boolean", false, 1)
	add("List", "contains", "Boolean", false, 1)
	add("List", "isEmpty", "Boolean", false, 0)
	add("List", "size", "Integer", false, 0)
	add("List", "indexOf", "Integer", false, 1)
	add("String", "valueOf", "String", true, 1)
	add("String", "join", "String", true, 2)
	add("String", "equals", "Boolean", false, 1)
	add("String", "equalsIgnoreCase", "Boolean", false, 1)
	add("String", "isEmpty", "Boolean", false, 0)
	add("String", "length", "Integer", false, 0)
	add("String", "contains", "Boolean", false, 1)
	add("String", "startsWith", "Boolean", false, 1, 2)
	add("String", "endsWith", "Boolean", false, 1)
	add("String", "substring", "String", false, 1, 2)
	add("String", "trim", "String", false, 0)
	add("String", "strip", "String", false, 0)
	add("String", "toLowerCase", "String", false, 0)
	add("String", "toUpperCase", "String", false, 0)
	add("String", "replace", "String", false, 2)
	add("Integer", "parseInt", "Integer", true, 1)
	add("Integer", "valueOf", "Integer", true, 1)
	add("Integer", "toString", "String", true, 1)
	add("Integer", "intValue", "Integer", false, 0)
	add("Long", "parseLong", "Long", true, 1)
	add("Long", "valueOf", "Long", true, 1)
	add("Long", "toString", "String", true, 1)
	add("Long", "longValue", "Long", false, 0)
	add("Double", "parseDouble", "Double", true, 1)
	add("Double", "valueOf", "Double", true, 1)
	add("Double", "toString", "String", true, 1)
	add("Double", "doubleValue", "Double", false, 0)
	add("Boolean", "valueOf", "Boolean", true, 1)
	add("Boolean", "parseBoolean", "Boolean", true, 1)
	add("Boolean", "toString", "String", true, 1)
	add("StringBuilder", "append", "StringBuilder", false, 1)
	add("StringBuilder", "length", "Integer", false, 0)
	add("StringBuilder", "toString", "String", false, 0)
	add("Math", "abs", "Number", true, 1)
	add("Math", "max", "Number", true, 2)
	add("Math", "min", "Number", true, 2)
	return table
}

var javaPureMethods = javaPureMethodAuthorities()

func javaTaskNeutralAuthorities() map[string]struct{} {
	values := []string{
		"Map", "List", "String", "Object", "Number", "Integer", "Long",
		"Double", "Float", "Boolean", "Short", "Byte", "Character",
		"Math", "StringBuilder", "CharSequence", "Iterable", "Comparable",
		"IllegalArgumentException", "IllegalStateException", "ArithmeticException",
	}
	authorities := make(map[string]struct{}, len(values))
	for _, value := range values {
		authorities[value] = struct{}{}
	}
	return authorities
}

func javaSourceIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if index == 0 && !unicode.IsLetter(char) && char != '_' && char != '$' {
			return false
		}
		if index > 0 && !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' && char != '$' {
			return false
		}
	}
	return true
}
