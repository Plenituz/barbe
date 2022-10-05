package core

import (
	"barbe/core/state_display"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"reflect"
	"strings"
	"time"
)

type Maker struct {
	OutputDir       string
	Parsers         []Parser
	PreTransformers []Transformer
	Templaters      []TemplateEngine
	Transformers    []Transformer
	Formatters      []Formatter
	Appliers        []Applier
}

type Executable struct {
	Message []string
	Files   []FileDescription
	Steps   []ExecutableStep
}
type ExecutableStep struct {
	Templates []FileDescription
}

type SyntaxTokenType = string

const (
	TokenTypeLiteralValue   SyntaxTokenType = "literal_value"
	TokenTypeScopeTraversal SyntaxTokenType = "scope_traversal"
	TokenTypeFunctionCall   SyntaxTokenType = "function_call"
	TokenTypeTemplate       SyntaxTokenType = "template"
	TokenTypeObjectConst    SyntaxTokenType = "object_const"
	TokenTypeArrayConst     SyntaxTokenType = "array_const"
	TokenTypeIndexAccess    SyntaxTokenType = "index_access"
	/*
		tuple = [for i, v in list: upper(v) if i > 2]
		object = {for k, v in map: k => upper(v)}
		object_of_tuples = {for v in list: v.key: v...}
	*/
	TokenTypeFor               SyntaxTokenType = "for"
	TokenTypeRelativeTraversal SyntaxTokenType = "relative_traversal"
	TokenTypeConditional       SyntaxTokenType = "conditional"
	TokenTypeBinaryOp          SyntaxTokenType = "binary_op"
	TokenTypeUnaryOp           SyntaxTokenType = "unary_op"
	TokenTypeParens            SyntaxTokenType = "parens"
	TokenTypeSplat             SyntaxTokenType = "splat"
	TokenTypeAnonymous         SyntaxTokenType = "anon"
)

var TokenTypes = map[SyntaxTokenType]struct{}{
	TokenTypeLiteralValue:      {},
	TokenTypeScopeTraversal:    {},
	TokenTypeFunctionCall:      {},
	TokenTypeTemplate:          {},
	TokenTypeObjectConst:       {},
	TokenTypeArrayConst:        {},
	TokenTypeIndexAccess:       {},
	TokenTypeFor:               {},
	TokenTypeRelativeTraversal: {},
	TokenTypeConditional:       {},
	TokenTypeBinaryOp:          {},
	TokenTypeUnaryOp:           {},
	TokenTypeParens:            {},
	TokenTypeSplat:             {},
	TokenTypeAnonymous:         {},
}

type TraverseType = string

const (
	TraverseTypeAttr  TraverseType = "attr"
	TraverseTypeIndex TraverseType = "index"
	TraverseTypeSplat TraverseType = "splat"
)

type Traverse struct {
	Type TraverseType

	//if TraverseTypeAttr
	Name *string `json:",omitempty"`

	//if TraverseTypeIndex
	//can be either a int64 or a string
	Index interface{} `json:",omitempty"`
}

type ObjectConstItem struct {
	Key   string
	Value SyntaxToken
}

func TokenPtr(s SyntaxToken) *SyntaxToken {
	return &s
}

type SyntaxToken struct {
	Type SyntaxTokenType

	//if TokenTypeLiteralValue
	Value interface{} `json:",omitempty"`

	// can be used by any type, used to carry extra metadata if needed
	Meta map[string]interface{} `json:",omitempty"`

	//if TokenTypeObjectConst
	//we dont support having expression for key names yet
	//CARE: this may contain several time the same key, on purpose
	//for example when several of the same blocks are merged together.
	//this needs to be taken into account by the formatter
	ObjectConst []ObjectConstItem `json:",omitempty"`

	//if TokenTypeArrayConst
	ArrayConst []SyntaxToken `json:",omitempty"`

	//if TokenTypeScopeTraversal TokenTypeRelativeTraversal
	Traversal []Traverse `json:",omitempty"`

	//if TokenTypeFunctionCall
	FunctionName *string       `json:",omitempty"`
	FunctionArgs []SyntaxToken `json:",omitempty"`

	//if TokenTypeTemplate
	Parts []SyntaxToken `json:",omitempty"`

	//if TokenTypeIndexAccess
	IndexCollection *SyntaxToken `json:",omitempty"`
	IndexKey        *SyntaxToken `json:",omitempty"`

	//if TokenTypeRelativeTraversal and TokenTypeParens and TokenTypeSplat
	Source *SyntaxToken `json:",omitempty"`

	//if TokenTypeFor
	ForKeyVar   *string      `json:",omitempty"` // empty if ignoring the key
	ForValVar   *string      `json:",omitempty"`
	ForCollExpr *SyntaxToken `json:",omitempty"`
	ForKeyExpr  *SyntaxToken `json:",omitempty"` // nil when producing a tuple
	ForValExpr  *SyntaxToken `json:",omitempty"`
	ForCondExpr *SyntaxToken `json:",omitempty"` // null if no "if" clause is present

	//if TokenTypeConditional
	Condition   *SyntaxToken `json:",omitempty"`
	TrueResult  *SyntaxToken `json:",omitempty"`
	FalseResult *SyntaxToken `json:",omitempty"`

	//if TokenTypeBinaryOp and TokenTypeUnaryOp
	RightHandSide *SyntaxToken `json:",omitempty"`
	Operator      *string      `json:",omitempty"`
	//if TokenTypeBinaryOp
	LeftHandSide *SyntaxToken `json:",omitempty"`

	//if TokenTypeSplat
	SplatEach *SyntaxToken `json:",omitempty"`
}

func (t *SyntaxToken) MergeWith(other SyntaxToken) error {
	if t.Type != other.Type {
		*t = other
		return nil
	}
	switch t.Type {
	default:
		*t = other
	case TokenTypeArrayConst:
		t.ArrayConst = append(t.ArrayConst, other.ArrayConst...)
	case TokenTypeObjectConst:
		existingValues := map[string]int{}
		for i, o := range t.ObjectConst {
			existingValues[o.Key] = i
		}
		for _, pairOther := range other.ObjectConst {
			if pairOther.Key == "target_tracking_scaling_policy_configuration" {
				log.Info().Msgf("%#v", pairOther)
			}
			if index, ok := existingValues[pairOther.Key]; ok {
				err := t.ObjectConst[index].Value.MergeWith(pairOther.Value)
				if err != nil {
					return errors.Wrap(err, "error merging key '"+pairOther.Key+"'")
				}
				continue
			}
			t.ObjectConst = append(t.ObjectConst, pairOther)
		}
	}
	return nil
}

type DataBag struct {
	Name   string
	Type   string
	Labels []string
	Value  SyntaxToken
}

func (d *DataBag) MergeWith(other DataBag) error {
	if d.Type != other.Type {
		return errors.New("cannot merge data bags with different types")
	}
	if d.Name != other.Name {
		return errors.New("cannot merge data bags with different names")
	}
	if d.Value.Type == "" {
		*d = other
		return nil
	}
	err := d.Value.MergeWith(other.Value)
	if err != nil {
		return errors.Wrap(err, "error merging databag value")
	}
	return nil
}

type DataBagGroup []*DataBag

func (d DataBagGroup) MergeWith(other DataBagGroup) (DataBagGroup, error) {
	groupByLabels := make(map[string]*DataBag)
	//TODO early out if all item have the same mergedLabel
	for _, bag := range d {
		mergedLabel := strings.Join(bag.Labels, "")
		if v, ok := groupByLabels[mergedLabel]; ok {
			err := v.MergeWith(*bag)
			if err != nil {
				return nil, err
			}
		} else {
			v := bag
			groupByLabels[mergedLabel] = v
		}
	}
	for _, bag := range other {
		mergedLabel := strings.Join(bag.Labels, "")
		if v, ok := groupByLabels[mergedLabel]; ok {
			err := v.MergeWith(*bag)
			if err != nil {
				return nil, err
			}
		} else {
			v := bag
			groupByLabels[mergedLabel] = v
		}
	}
	output := make(DataBagGroup, 0, len(groupByLabels))
	for _, v := range groupByLabels {
		output = append(output, v)
	}
	return output, nil
}

type ConfigContainer struct {
	DataBags map[string] /*type*/ map[string] /*name*/ DataBagGroup
}

func (c *ConfigContainer) GetDataBagsOfType(bagType string) []*DataBag {
	if m, ok := c.DataBags[bagType]; ok {
		arr := make([]*DataBag, 0, len(m))
		for _, v := range m {
			arr = append(arr, v...)
		}
		return arr
	}
	return []*DataBag{}
}

func (c *ConfigContainer) GetDataBagGroup(bagType string, bagName string) DataBagGroup {
	if m, ok := c.DataBags[bagType]; ok {
		if group, ok := m[bagName]; ok {
			return group
		}
	}
	return nil
}

func (c *ConfigContainer) Insert(bag DataBag) error {
	if _, ok := c.DataBags[bag.Type]; !ok {
		c.DataBags[bag.Type] = make(map[string]DataBagGroup)
	}
	if _, ok := c.DataBags[bag.Type][bag.Name]; !ok {
		c.DataBags[bag.Type][bag.Name] = make(DataBagGroup, 0, 1)
	}
	var err error
	c.DataBags[bag.Type][bag.Name], err = c.GetDataBagGroup(bag.Type, bag.Name).MergeWith([]*DataBag{&bag})
	return err
}

type FileDescription struct {
	Name    string
	Content []byte
}

type Parser interface {
	Name() string
	CanParse(ctx context.Context, fileDesc FileDescription) (bool, error)
	// Parse parses the file and returns only the part of the data it understands
	Parse(ctx context.Context, fileDesc FileDescription, container *ConfigContainer) error
}

type TemplateEngine interface {
	Name() string
	Apply(ctx context.Context, container *ConfigContainer, templates []FileDescription) error
}

type Transformer interface {
	Name() string
	Transform(ctx context.Context, container *ConfigContainer) error
}

type Formatter interface {
	Name() string
	// Format formats the data from the parsed data in common format
	Format(ctx context.Context, container *ConfigContainer) error
}

type Applier interface {
	Name() string
	Apply(ctx context.Context, container *ConfigContainer) error
}

func (maker *Maker) Make(ctx context.Context, inputFiles []FileDescription, apply bool) (*ConfigContainer, error) {
	container := &ConfigContainer{
		DataBags: map[string]map[string]DataBagGroup{},
	}

	err := maker.ParseFiles(ctx, inputFiles, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing input files")
	}

	t := time.Now()
	state_display.StartMajorStep("Fetch templates")
	executable, err := GetTemplates(ctx, container)
	state_display.EndMajorStep("Fetch templates")
	log.Ctx(ctx).Debug().Msgf("getting templates took: %s", time.Since(t))
	if err != nil {
		return container, errors.Wrap(err, "error getting templates")
	}

	if len(executable.Message) != 0 {
		log.Ctx(ctx).Info().Msg(strings.Join(executable.Message, "\n"))
	}

	err = maker.ParseFiles(ctx, executable.Files, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing files from manifest")
	}

	state_display.StartMajorStep("Pre-transform")
	err = maker.PreTransform(ctx, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Pre-transform")

	for i, step := range executable.Steps {

		stepName := fmt.Sprintf("Step %d", i+1)
		state_display.StartMajorStep(stepName)
		log.Ctx(ctx).Debug().Msgf("executing step %d", i)

		for _, engine := range maker.Templaters {
			state_display.StartMinorStep(stepName, engine.Name())
			log.Ctx(ctx).Debug().Msg("applying template engine: " + engine.Name())
			t := time.Now()
			
			err = engine.Apply(ctx, container, step.Templates)

			state_display.EndMinorStep(stepName, engine.Name())
			log.Ctx(ctx).Debug().Msgf("template engine '%s' took: %v", engine.Name(), time.Since(t))

			if err != nil {
				return container, errors.Wrap(err, "from template engine '"+engine.Name()+"'")
			}
		}
		err = maker.Transform(ctx, container, stepName)
		if err != nil {
			return container, err
		}
		state_display.EndMajorStep(stepName)
	}

	state_display.StartMajorStep("Formatters")
	for _, formatter := range maker.Formatters {
		log.Ctx(ctx).Debug().Msgf("formatting %s", formatter.Name())
		err := formatter.Format(ctx, container)
		if err != nil {
			return container, err
		}
	}
	state_display.EndMajorStep("Formatters")

	if apply {
		state_display.StartMajorStep("Appliers")
		for _, applier := range maker.Appliers {
			state_display.StartMinorStep("Appliers", applier.Name())
			log.Ctx(ctx).Debug().Msgf("applying %s", applier.Name())
			err := applier.Apply(ctx, container)
			state_display.EndMinorStep("Appliers", applier.Name())
			if err != nil {
				return container, err
			}
		}
		state_display.EndMajorStep("Appliers")
	}
	return container, nil
}

func (maker *Maker) ParseFiles(ctx context.Context, files []FileDescription, container *ConfigContainer) error {
	for _, file := range files {
		for _, parser := range maker.Parsers {
			canParse, err := parser.CanParse(ctx, file)
			if err != nil {
				return err
			}
			if !canParse {
				continue
			}
			log.Ctx(ctx).Debug().Msgf("parsing %s with %s", file.Name, parser.Name())
			err = parser.Parse(ctx, file, container)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (maker *Maker) PreTransform(ctx context.Context, container *ConfigContainer) error {
	for _, transformer := range maker.PreTransformers {
		log.Ctx(ctx).Debug().Msgf("applying pre-transformer '%s'", transformer.Name())
		t := time.Now()
		err := transformer.Transform(ctx, container)
		log.Ctx(ctx).Debug().Msgf("pre-transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return err
		}
	}
	return nil
}

func (maker *Maker) Transform(ctx context.Context, container *ConfigContainer, displayName string) error {
	for _, transformer := range maker.Transformers {
		state_display.StartMinorStep(displayName, transformer.Name())
		log.Ctx(ctx).Debug().Msgf("applying transformer '%s'", transformer.Name())
		t := time.Now()
		err := transformer.Transform(ctx, container)
		state_display.EndMinorStep(displayName, transformer.Name())
		log.Ctx(ctx).Debug().Msgf("transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return err
		}
	}
	return nil
}

func InterfaceIsNil(i interface{}) bool {
	if i == nil {
		return true
	}

	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	}

	return false
}
