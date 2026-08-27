package gen

import (
	"go/parser"
	"go/token"
	"testing"

	liquidv1 "github.com/candacelabs/candace/pkg/liquidproto/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestGenerator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Liquid Proto generator")
}

var _ = Describe("Liquid Proto generator", func() {
	It("emits a direct message validator", func() {
		digestOptions := new(descriptorpb.FieldOptions)
		proto.SetExtension(digestOptions, liquidv1.E_Field, &liquidv1.FieldRefinement{
			Expr: "len(this) == 32",
		})
		desiredStateOptions := new(descriptorpb.FieldOptions)
		proto.SetExtension(desiredStateOptions, liquidv1.E_Field, &liquidv1.FieldRefinement{
			Expr: "this == 1 || this == 2",
		})
		fixture := &descriptorpb.FileDescriptorProto{
			Name:       proto.String("liquid_bytes_fixture.proto"),
			Package:    proto.String("candace.liquid.fixture.v1"),
			Syntax:     proto.String("proto3"),
			Dependency: []string{"liquidproto/v1/refinement.proto"},
			Options: &descriptorpb.FileOptions{
				GoPackage: proto.String("example.com/liquidfixture;liquidfixture"),
			},
			EnumType: []*descriptorpb.EnumDescriptorProto{{
				Name: proto.String("DesiredState"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("DESIRED_STATE_UNSPECIFIED"), Number: proto.Int32(0)},
					{Name: proto.String("DESIRED_STATE_RUNNING"), Number: proto.Int32(1)},
					{Name: proto.String("DESIRED_STATE_STOPPED"), Number: proto.Int32(2)},
				},
			}},
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Blob"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:    proto.String("digest"),
						Number:  proto.Int32(1),
						Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:    descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
						Options: digestOptions,
					},
					{
						Name:     proto.String("desired_state"),
						Number:   proto.Int32(2),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: proto.String(".candace.liquid.fixture.v1.DesiredState"),
						Options:  desiredStateOptions,
					},
				},
			}},
		}
		request := &pluginpb.CodeGeneratorRequest{
			FileToGenerate: []string{fixture.GetName()},
			ProtoFile: []*descriptorpb.FileDescriptorProto{
				protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
				protodesc.ToFileDescriptorProto(liquidv1.File_liquidproto_v1_refinement_proto),
				fixture,
			},
		}
		plugin, err := (protogen.Options{}).New(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(Run(plugin)).To(Succeed())

		files := plugin.Response().GetFile()
		Expect(files).To(HaveLen(1))
		generated := files[0].GetContent()
		for _, want := range []string{
			"func ValidateBlob(message *Blob) error",
			"len(message.Digest) == 32",
			"message.DesiredState == 1 || message.DesiredState == 2",
			`Field:     "desired_state"`,
		} {
			Expect(generated).To(ContainSubstring(want))
		}
		Expect(generated).To(MatchRegexp(`Value:\s+message[.]Digest`))
		Expect(generated).To(MatchRegexp(`Value:\s+message[.]DesiredState`))
		for _, unwanted := range []string{"RefinedBlob", "MustBlob", "ToProto"} {
			Expect(generated).NotTo(ContainSubstring(unwanted))
		}

		_, err = parser.ParseFile(token.NewFileSet(), files[0].GetName(), generated, parser.AllErrors)
		Expect(err).NotTo(HaveOccurred(), "generated source is not valid Go syntax:\n%s", generated)
	})

	It("emits deterministic string-map entry validation", func() {
		labelsOptions := new(descriptorpb.FieldOptions)
		proto.SetExtension(labelsOptions, liquidv1.E_Field, &liquidv1.FieldRefinement{
			MapKeyExpr:   "matches(this, `^[a-z]+$`)",
			MapValueExpr: "len(this) >= 1",
		})
		fixture := &descriptorpb.FileDescriptorProto{
			Name:       proto.String("liquid_map_fixture.proto"),
			Package:    proto.String("candace.liquid.fixture.v1"),
			Syntax:     proto.String("proto3"),
			Dependency: []string{"liquidproto/v1/refinement.proto"},
			Options: &descriptorpb.FileOptions{
				GoPackage: proto.String("example.com/liquidfixture;liquidfixture"),
			},
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Node"),
				NestedType: []*descriptorpb.DescriptorProto{{
					Name:    proto.String("LabelsEntry"),
					Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name: proto.String("key"), Number: proto.Int32(1),
							Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						},
						{
							Name: proto.String("value"), Number: proto.Int32(2),
							Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							Type:  descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						},
					},
				}},
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name: proto.String("labels"), Number: proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".candace.liquid.fixture.v1.Node.LabelsEntry"),
					Options:  labelsOptions,
				}},
			}},
		}
		request := &pluginpb.CodeGeneratorRequest{
			FileToGenerate: []string{fixture.GetName()},
			ProtoFile: []*descriptorpb.FileDescriptorProto{
				protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
				protodesc.ToFileDescriptorProto(liquidv1.File_liquidproto_v1_refinement_proto),
				fixture,
			},
		}
		plugin, err := (protogen.Options{}).New(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(Run(plugin)).To(Succeed())

		files := plugin.Response().GetFile()
		Expect(files).To(HaveLen(1))
		generated := files[0].GetContent()
		for _, want := range []string{
			"func ValidateNode(message *Node) error",
			"sort.Strings(_liquidNodeLabelsKeys)",
			"_liquidNodeLabelsKeyRe0.MatchString(_liquidNodeLabelsKey)",
			"len(_liquidNodeLabelsValue) >= 1",
			`Field:     "labels"`,
		} {
			Expect(generated).To(ContainSubstring(want))
		}
		_, err = parser.ParseFile(token.NewFileSet(), files[0].GetName(), generated, parser.AllErrors)
		Expect(err).NotTo(HaveOccurred(), "generated source is not valid Go syntax:\n%s", generated)
	})
})
