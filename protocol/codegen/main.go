package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// @sk-task kvn-android#T1.1: protocol codegen entrypoint (AC-004)
func main() {
	repoRoot := findRepoRoot()
	protoDir := filepath.Join(repoRoot, "protocol")

	framesPath := filepath.Join(protoDir, "frames.yaml")
	handshakePath := filepath.Join(protoDir, "handshake.yaml")

	framesSpec := loadFrames(framesPath)
	handshakeSpec := loadHandshake(handshakePath)

	goDir := filepath.Join(repoRoot, "src")
	kotlinDir := filepath.Join(repoRoot, "src/android/app/src/main/kotlin/com/kvn/client/protocol")

	// Generate Go framing types
	goFraming := filepath.Join(goDir, framesSpec.GoPkgRel())
	os.MkdirAll(goFraming, 0755)
	writeFile(filepath.Join(goFraming, "types_gen.go"), generateGoFraming(framesSpec))

	// Generate Go handshake types
	goHandshake := filepath.Join(goDir, handshakeSpec.GoPkgRel())
	os.MkdirAll(goHandshake, 0755)
	writeFile(filepath.Join(goHandshake, "types_gen.go"), generateGoHandshake(handshakeSpec))

	// Generate Kotlin data classes
	os.MkdirAll(kotlinDir, 0755)
	writeFile(filepath.Join(kotlinDir, "Frames.kt"), generateKotlinFrames(framesSpec, handshakeSpec))
	writeFile(filepath.Join(kotlinDir, "Handshake.kt"), generateKotlinHandshake(handshakeSpec))

	fmt.Println("Codegen done. Files written to:")
	fmt.Printf("  %s\n", filepath.Join(goFraming, "types_gen.go"))
	fmt.Printf("  %s\n", filepath.Join(goHandshake, "types_gen.go"))
	fmt.Printf("  %s\n", filepath.Join(kotlinDir, "Frames.kt"))
	fmt.Printf("  %s\n", filepath.Join(kotlinDir, "Handshake.kt"))
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func loadFrames(path string) *FramesSpec {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	var spec FramesSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	return &spec
}

func loadHandshake(path string) *HandshakeSpec {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	var spec HandshakeSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	return &spec
}

func writeFile(path string, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
}

type FramesSpec struct {
	Package       string            `yaml:"package"`
	GoPkg         string            `yaml:"go_package"`
	KotlinPkg     string            `yaml:"kotlin_package"`
	FrameTypes    map[string]int    `yaml:"frame_types"`
	FrameFlags    map[string]int    `yaml:"frame_flags"`
	Frame         FrameDef          `yaml:"frame"`
}

func (s *FramesSpec) GoPkgRel() string {
	// "github.com/bzdvdn/kvn-ws/src/internal/transport/framing" -> "internal/transport/framing"
	parts := strings.SplitN(s.GoPkg, "/kvn-ws/", 2)
	if len(parts) == 2 {
		// strip leading "src/" since we join with goDir which already includes src
		rel := strings.TrimPrefix(parts[1], "src/")
		return rel
	}
	return s.Package
}

type FrameDef struct {
	Name           string          `yaml:"name"`
	HeaderSize     int             `yaml:"header_size"`
	MaxPayloadSize int             `yaml:"max_payload_size"`
	Fields         []FrameFieldDef `yaml:"fields"`
}

type FrameFieldDef struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Offset int    `yaml:"offset"`
}

type HandshakeSpec struct {
	Package        string           `yaml:"package"`
	GoPkg          string           `yaml:"go_package"`
	KotlinPkg      string           `yaml:"kotlin_package"`
	ProtoVersion   int              `yaml:"protocol_version"`
	SessionIDLen   int              `yaml:"session_id_len"`
	Constants      []ConstDef       `yaml:"constants"`
	ClientHello    MessageDef       `yaml:"client_hello"`
	ServerHello    MessageDef       `yaml:"server_hello"`
	AuthError      MessageDef       `yaml:"auth_error"`
}

func (s *HandshakeSpec) GoPkgRel() string {
	parts := strings.SplitN(s.GoPkg, "/kvn-ws/", 2)
	if len(parts) == 2 {
		rel := strings.TrimPrefix(parts[1], "src/")
		return rel
	}
	return s.Package
}

type ConstDef struct {
	Name  string `yaml:"name"`
	Type  string `yaml:"type"`
	Value int    `yaml:"value"`
}

type MessageDef struct {
	Name   string       `yaml:"name"`
	Fields []FieldDef   `yaml:"fields"`
}

type FieldDef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

func generateGoFraming(spec *FramesSpec) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("// Code generated from protocol/frames.yaml. DO NOT EDIT.\n"))
	b.WriteString(fmt.Sprintf("package %s\n\n", spec.Package))
	b.WriteString("import \"errors\"\n\n")

	// Frame type constants
	b.WriteString(fmt.Sprintf("// @sk-task kvn-android#T1.1: protocol frame types (AC-004)\n"))
	b.WriteString("const (\n")
	for _, name := range sortedKeys(spec.FrameTypes) {
		b.WriteString(fmt.Sprintf("\t%s = 0x%02X\n", name, spec.FrameTypes[name]))
	}
	b.WriteString("\n")
	// Frame flags
	for _, name := range sortedKeys(spec.FrameFlags) {
		b.WriteString(fmt.Sprintf("\t%s = 0x%02X\n", name, spec.FrameFlags[name]))
	}
	b.WriteString(fmt.Sprintf("\n\tFrameMaxPayloadSize = %d\n", spec.Frame.MaxPayloadSize))
	b.WriteString(fmt.Sprintf("\tFrameHeaderSize     = %d\n", spec.Frame.HeaderSize))
	b.WriteString(")\n\n")

	// Frame struct
	// Error vars
	b.WriteString("// @sk-task kvn-android#T1.1: protocol error vars (AC-004)\n")
	b.WriteString("var (\n")
	b.WriteString("\tErrPayloadTooLarge = errors.New(\"payload exceeds max frame size\")\n")
	b.WriteString("\tErrFrameTooShort = errors.New(\"frame too short\")\n")
	b.WriteString(")\n\n")

	// Frame struct (type only, behavioral methods stay in manual framing.go)
	b.WriteString("// @sk-task kvn-android#T1.1: generated Frame type (AC-004)\n")
	b.WriteString(fmt.Sprintf("type %s struct {\n", spec.Frame.Name))
	for _, f := range spec.Frame.Fields {
		goType := yamlTypeToGo(f.Type)
		b.WriteString(fmt.Sprintf("\t%s %s\n", f.Name, goType))
	}
	b.WriteString("}\n\n")

	return b.String()
}

func generateGoHandshake(spec *HandshakeSpec) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("// Code generated from protocol/handshake.yaml. DO NOT EDIT.\n"))
	b.WriteString(fmt.Sprintf("package %s\n\n", spec.Package))
	b.WriteString("import \"net\"\n\n")

	// Constants
	b.WriteString("// @sk-task kvn-android#T1.1: protocol handshake constants (AC-004)\n")
	b.WriteString("const (\n")
	for _, c := range spec.Constants {
		switch c.Type {
		case "byte":
			b.WriteString(fmt.Sprintf("\t%s = 0x%02X\n", c.Name, c.Value))
		default:
			b.WriteString(fmt.Sprintf("\t%s = %d\n", c.Name, c.Value))
		}
	}
	b.WriteString(fmt.Sprintf("\tProtoVersion = 0x%02X\n", spec.ProtoVersion))
	b.WriteString(fmt.Sprintf("\tSessionIDLen = %d\n", spec.SessionIDLen))
	b.WriteString(")\n\n")

	// ClientHello
	genMessage := func(m MessageDef) string {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("// @sk-task kvn-android#T1.1: generated %s (AC-004)\n", m.Name))
		sb.WriteString(fmt.Sprintf("type %s struct {\n", m.Name))
		for _, f := range m.Fields {
			goType := yamlFieldToGo(f.Type)
			sb.WriteString(fmt.Sprintf("\t%s %s\n", f.Name, goType))
		}
		sb.WriteString("}\n")
		return sb.String()
	}

	b.WriteString(genMessage(spec.ClientHello))
	b.WriteString(genMessage(spec.ServerHello))
	b.WriteString(genMessage(spec.AuthError))

	return b.String()
}

func generateKotlinFrames(frames *FramesSpec, handshake *HandshakeSpec) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("// Code generated from protocol/frames.yaml. DO NOT EDIT.\n"))
	b.WriteString(fmt.Sprintf("package %s\n\n", frames.KotlinPkg))

	b.WriteString("// @sk-task kvn-android#T1.1: Kotlin frame type constants (AC-004)\n")
	b.WriteString("object FrameTypes {\n")
	for _, name := range sortedKeys(frames.FrameTypes) {
		b.WriteString(fmt.Sprintf("\tconst val %s: Byte = %d.toByte()\n", kotlinConstName(name), int(byte(frames.FrameTypes[name]))))
	}
	b.WriteString("}\n\n")

	b.WriteString("// @sk-task kvn-android#T1.1: Kotlin frame flag constants (AC-004)\n")
	b.WriteString("object FrameFlags {\n")
	for _, name := range sortedKeys(frames.FrameFlags) {
		b.WriteString(fmt.Sprintf("\tconst val %s: Byte = %d.toByte()\n", kotlinConstName(name), int(byte(frames.FrameFlags[name]))))
	}
	b.WriteString("}\n\n")

	// Frame data class (Length is computed from payload.size, not stored)
	b.WriteString("// @sk-task kvn-android#T1.1: Kotlin Frame data class (AC-004)\n")
	b.WriteString("data class Frame(\n")
	b.WriteString("\tval type: Byte,\n")
	b.WriteString("\tval flags: Byte,\n")
	b.WriteString("\tval payload: ByteArray\n")
	b.WriteString(") {\n")
	b.WriteString("\tval length: Int get() = payload.size\n")
	b.WriteString("}")

	return b.String()
}

func generateKotlinHandshake(spec *HandshakeSpec) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("// Code generated from protocol/handshake.yaml. DO NOT EDIT.\n"))
	b.WriteString(fmt.Sprintf("package %s\n\n", spec.KotlinPkg))

	// @sk-task kvn-android#T5.19: Kotlin handshake constants (AC-004)
	b.WriteString("// @sk-task kvn-android#T1.1: protocol handshake constants (AC-004)\n")
	for _, c := range spec.Constants {
		ktName := kotlinConstHandshake(c.Name)
		switch c.Type {
		case "byte":
			b.WriteString(fmt.Sprintf("const val %s: Byte = 0x%02X.toByte()\n", ktName, c.Value))
		default:
			b.WriteString(fmt.Sprintf("const val %s: Int = %d\n", ktName, c.Value))
		}
	}
	b.WriteString(fmt.Sprintf("const val PROTO_VERSION: Byte = 0x%02X.toByte()\n", spec.ProtoVersion))
	b.WriteString(fmt.Sprintf("const val SESSION_ID_LEN: Int = %d\n\n", spec.SessionIDLen))

	for _, msg := range []MessageDef{spec.ClientHello, spec.ServerHello, spec.AuthError} {
		b.WriteString(fmt.Sprintf("// @sk-task kvn-android#T1.1: Kotlin %s data class (AC-004)\n", msg.Name))
		b.WriteString(fmt.Sprintf("data class %s(\n", msg.Name))
		for i, f := range msg.Fields {
			comma := ","
			if i == len(msg.Fields)-1 {
				comma = ""
			}
			ktType := yamlFieldToKotlin(f.Type)
			b.WriteString(fmt.Sprintf("\tval %s: %s%s\n", lowerFirst(f.Name), ktType, comma))
		}
		b.WriteString(")\n\n")
	}

	return b.String()
}

// @sk-task kvn-android#T5.19: generate UPPER_SNAKE_CASE for handshake constants (AC-004)
func kotlinConstHandshake(name string) string {
	// Handle known patterns with special cases like FlagIPv6 -> FLAG_IPV6
	// Standard conversion: insert underscore before each uppercase letter
	// that follows a lowercase letter or another uppercase followed by lowercase.
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := name[i-1]
			// Add underscore if previous is lowercase
			if prev >= 'a' && prev <= 'z' {
				result.WriteRune('_')
			} else if prev >= 'A' && prev <= 'Z' {
				// Check if this is continuation of an acronym (e.g., "IPv" in "FlagIPv6")
				// Don't split if next char is lowercase followed by digit (e.g., "v6")
				if i+2 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z' && name[i+2] >= '0' && name[i+2] <= '9' {
					// skip - part of acronym like "IPv6"
				} else if i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z' {
					result.WriteRune('_')
				}
			}
		}
		result.WriteRune(r)
	}
	return strings.ToUpper(result.String())
}

func yamlTypeToGo(t string) string {
	switch t {
	case "byte":
		return "byte"
	case "bytes":
		return "[]byte"
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "uint16":
		return "uint16"
	case "string":
		return "string"
	case "ip":
		return "net.IP"
	default:
		return t
	}
}

func yamlFieldToGo(t string) string {
	switch t {
	case "byte":
		return "byte"
	case "bytes":
		return "[]byte"
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "uint16":
		return "uint16"
	case "string":
		return "string"
	case "ip":
		return "net.IP"
	default:
		return t
	}
}

func yamlFieldToKotlin(t string) string {
	switch t {
	case "byte":
		return "Byte"
	case "bytes":
		return "ByteArray"
	case "bool":
		return "Boolean"
	case "int":
		return "Int"
	case "uint16":
		return "UShort"
	case "string":
		return "String"
	case "ip":
		return "String" // Represent IP as string in Kotlin
	default:
		return t
	}
}

func kotlinConstName(name string) string {
	// FrameTypeData -> FRAME_TYPE_DATA, FrameTypeDNS -> FRAME_TYPE_DNS
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prevIsUpper := name[i-1] >= 'A' && name[i-1] <= 'Z'
			nextIsLower := i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z'
			if !prevIsUpper || nextIsLower {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}
	return strings.ToUpper(result.String())
}

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(string(s[0])) + s[1:]
}

func sortedKeys(m interface{}) []string {
	var keys []string
	switch v := m.(type) {
	case map[string]int:
		for k := range v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
