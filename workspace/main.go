package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	fmt.Println("Applying patches...")
	if err := applyPatches(); err != nil {
		fmt.Printf("Error applying patches: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Patches applied successfully!")
}

func applyPatches() error {
	patches := []struct {
		path    string
		find    string
		replace string
	}{
		{
			path: "internal/descriptor/registry.go",
			find: "type Registry struct {",
			replace: "type Registry struct {\n\toperationNamingScheme string",
		},
		{
			path: "internal/descriptor/registry.go",
			find: "func NewRegistry() *Registry {",
			replace: "func (r *Registry) SetOperationNamingScheme(scheme string) {\n\tr.operationNamingScheme = scheme\n}\n\nfunc (r *Registry) GetOperationNamingScheme() string {\n\treturn r.operationNamingScheme\n}\n\nfunc NewRegistry() *Registry {",
		},
		{
			path: "protoc-gen-openapiv2/main.go",
			find: `importPrefix = flag.String("import_prefix"`,
			replace: `operationNamingScheme = flag.String("operation_naming_scheme", "legacy", "naming scheme to use for operationId (legacy, service_method, fqn)")
	importPrefix = flag.String("import_prefix"`,
		},
		{
			path: "protoc-gen-openapiv2/main.go",
			find: `genopenapi.New(reg)`,
			replace: `reg.SetOperationNamingScheme(*operationNamingScheme)
	genopenapi.New(reg)`,
		},
		{
			path: "protoc-gen-openapiv2/internal/genopenapi/template.go",
			find: "\top := swaggerOperationObject{\n\t\tTags:        []string{binding.Method.Service.GetName()},\n\t\tOperationId: binding.Method.GetName(),",
			replace: "\tvar operationID string\n\tscheme := t.GetOperationNamingScheme()\n\tswitch scheme {\n\tcase \"service_method\":\n\t\toperationID = fmt.Sprintf(\"%s_%s\", binding.Method.Service.GetName(), binding.Method.GetName())\n\tcase \"fqn\":\n\t\tpkg := binding.Method.Service.File.GetPackage()\n\t\tif pkg != \"\" {\n\t\t	operationID = fmt.Sprintf(\"%s.%s.%s\", pkg, binding.Method.Service.GetName(), binding.Method.GetName())\n\t\t} else {\n\t\t	operationID = fmt.Sprintf(\"%s.%s\", binding.Method.Service.GetName(), binding.Method.GetName())\n\t\t}\n\tcase \"legacy\", \"method\", \"\":\n\t\toperationID = binding.Method.GetName()\n\tdefault:\n\t\toperationID = binding.Method.GetName()\n\t}\n\n\top := swaggerOperationObject{\n\t\tTags:        []string{binding.Method.Service.GetName()},\n\t\tOperationId: operationID,",
		},
	}

	for _, p := range patches {
		content, err := ioutil.ReadFile(p.path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", p.path, err)
		}
		if strings.Contains(string(content), p.replace) {
			continue
		}
		if !strings.Contains(string(content), p.find) {
			return fmt.Errorf("target string not found in %s", p.path)
		}
		newContent := strings.Replace(string(content), p.find, p.replace, 1)
		err = ioutil.WriteFile(p.path, []byte(newContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s: %v", p.path, err)
		}
	}

	// Add unit test to generator_test.go
	testFile := "protoc-gen-openapiv2/internal/genopenapi/generator_test.go"
	testContent, err := ioutil.ReadFile(testFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", testFile, err)
	}
	if !strings.Contains(string(testContent), "TestOperationNamingScheme") {
		newTestContent := string(testContent)
		if !strings.Contains(newTestContent, "\"encoding/json\"") {
			newTestContent = strings.Replace(newTestContent, "import (", "import (\n\t\"encoding/json\"", 1)
		}
		unitTestCode := `

func TestOperationNamingScheme(t *testing.T) {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("test.proto"),
		Syntax:  proto.String("proto3"),
		Package: proto.String("test.v1"),
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("UserService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Get"),
						InputType:  proto.String(".test.v1.GetUserRequest"),
						OutputType: proto.String(".test.v1.GetUserResponse"),
					},
				},
			},
			{
				Name: proto.String("ProductService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Get"),
						InputType:  proto.String(".test.v1.GetProductRequest"),
						OutputType: proto.String(".test.v1.GetProductResponse"),
					},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("GetUserRequest")},
			{Name: proto.String("GetUserResponse")},
			{Name: proto.String("GetProductRequest")},
			{Name: proto.String("GetProductResponse")},
			{Name: proto.String("GetProductResponse")},
		},
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"test.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{file},
	}

	testCases := []struct {
		scheme   string
		expected map[string]string
	}{
		{
			scheme: "legacy",
			expected: map[string]string{
				"/test.v1.UserService/Get":    "Get",
				"/test.v1.ProductService/Get": "Get",
			},
		},
		{
			scheme: "service_method",
			expected: map[string]string{
				"/test.v1.UserService/Get":    "UserService_Get",
				"/test.v1.ProductService/Get": "ProductService_Get",
			},
		},
		{
			scheme: "fqn",
			expected: map[string]string{
				"/test.v1.UserService/Get":    "test.v1.UserService.Get",
				"/test.v1.ProductService/Get": "test.v1.ProductService.Get",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.scheme, func(t *testing.T) {
			reg := descriptor.NewRegistry()
			reg.SetOperationNamingScheme(tc.scheme)
			reg.SetGenerateUnboundMethods(true)
			if err := reg.Load(req); err != nil {
				t.Fatalf("reg.Load failed: %v", err)
			}

			g := New(reg)
			var targets []*descriptor.File
			for _, target := range req.FileToGenerate {
				f, err := reg.LookupFile(target)
				if err != nil {
					t.Fatal(err)
				}
				targets = append(targets, f)
			}
			files, err := g.Generate(targets)
			if err != nil {
				t.Fatalf("g.Generate failed: %v", err)
			}

			if len(files) == 0 {
				t.Fatal("no files generated")
			}
			content := files[0].GetContent()
			var doc struct {
				Paths map[string]map[string]struct {
					OperationId string "json:\"operationId\""
				} "json:\"paths\""
			}
			if err := json.Unmarshal([]byte(content), &doc); err != nil {
				t.Fatalf("failed to unmarshal generated JSON: %v", err)
			}

			for path, expectedOpID := range tc.expected {
				methods, ok := doc.Paths[path]
				if !ok {
					t.Fatalf("path %s not found in generated paths: %v", path, doc.Paths)
				}
				op, ok := methods["post"]
				if !ok {
					t.Fatalf("POST method not found for path %s", path)
				}
				if op.OperationId != expectedOpID {
					t.Errorf("for scheme %s, path %s: expected operationId %q, got %q", tc.scheme, path, expectedOpID, op.OperationId)
				}
			}
		})
	}
}
`
		newTestContent += unitTestCode
		err = ioutil.WriteFile(testFile, []byte(newTestContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s: %v", testFile, err)
		}
	}

	return nil
}
