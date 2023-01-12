package core

import (
	"barbe/core/fetcher"
	"context"
	"github.com/pkg/errors"
	"reflect"
	"strings"
	"sync"
)

type Executable struct {
	Message string
	//files are plain config files that are added to the files to parse
	Files      []fetcher.FileDescription
	Components []fetcher.FileDescription
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

	maxComponentLoops = 200
)

func IsTokenType(t string) bool {
	switch t {
	case TokenTypeLiteralValue,
		TokenTypeScopeTraversal,
		TokenTypeFunctionCall,
		TokenTypeTemplate,
		TokenTypeObjectConst,
		TokenTypeArrayConst,
		TokenTypeIndexAccess,
		TokenTypeFor,
		TokenTypeRelativeTraversal,
		TokenTypeConditional,
		TokenTypeBinaryOp,
		TokenTypeUnaryOp,
		TokenTypeParens,
		TokenTypeSplat,
		TokenTypeAnonymous:
		return true
	}
	return false
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

func Ptr[T any](s T) *T {
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

// a token A is the super set of another token B if A contains everything B does or more.
func (t SyntaxToken) IsSuperSetOf(other SyntaxToken) bool {
	if t.Type != other.Type {
		return false
	}
	switch t.Type {
	default:
		return false
	case TokenTypeObjectConst:
		merged, err := t.MergeWith(other)
		if err != nil {
			return false
		}
		return TokensDeepEqual(merged, t)
	}
}

func (t SyntaxToken) MergeWith(other SyntaxToken) (SyntaxToken, error) {
	if t.Type != other.Type {
		return other, nil
	}
	switch t.Type {
	default:
		return other, nil
	case TokenTypeObjectConst:
		existingValues := map[string]int{}
		for i, o := range t.ObjectConst {
			existingValues[o.Key] = i
		}
		for _, pairOther := range other.ObjectConst {
			if index, ok := existingValues[pairOther.Key]; ok {
				var err error
				t.ObjectConst[index].Value, err = t.ObjectConst[index].Value.MergeWith(pairOther.Value)
				if err != nil {
					return SyntaxToken{}, errors.Wrap(err, "error merging key '"+pairOther.Key+"'")
				}
				continue
			}
			t.ObjectConst = append(t.ObjectConst, pairOther)
		}
	}
	return t, nil
}

type DataBag struct {
	Name   string
	Type   string
	Labels []string
	Value  SyntaxToken
}

func (d DataBag) MergeWith(other DataBag) (DataBag, error) {
	if d.Type != other.Type {
		return other, errors.New("cannot merge data bags with different types")
	}
	if d.Name != other.Name {
		return other, errors.New("cannot merge data bags with different names")
	}
	if d.Value.Type == "" {
		return other, nil
	}
	var err error
	d.Value, err = d.Value.MergeWith(other.Value)
	if err != nil {
		return other, errors.Wrap(err, "error merging databag value")
	}
	return d, nil
}

type DataBagGroup []DataBag

func (d DataBagGroup) MergeWith(other DataBagGroup) (DataBagGroup, error) {
	var err error
	groupByLabels := make(map[string]DataBag)
	//TODO early out if all item have the same mergedLabel
	for _, bag := range d {
		mergedLabel := strings.Join(bag.Labels, "")
		if v, ok := groupByLabels[mergedLabel]; ok {
			groupByLabels[mergedLabel], err = v.MergeWith(bag)
			if err != nil {
				return nil, err
			}
		} else {
			groupByLabels[mergedLabel] = bag
		}
	}
	for _, bag := range other {
		mergedLabel := strings.Join(bag.Labels, "")
		if v, ok := groupByLabels[mergedLabel]; ok {
			groupByLabels[mergedLabel], err = v.MergeWith(bag)
			if err != nil {
				return nil, err
			}
		} else {
			groupByLabels[mergedLabel] = bag
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

func NewConfigContainer() *ConfigContainer {
	return &ConfigContainer{
		DataBags: make(map[string]map[string]DataBagGroup),
	}
}

func (c *ConfigContainer) Clone() *ConfigContainer {
	clone := NewConfigContainer()
	for dataType, dataBags := range c.DataBags {
		clone.DataBags[dataType] = make(map[string]DataBagGroup, len(dataBags))
		for dataName, dataBagGroup := range dataBags {
			clone.DataBags[dataType][dataName] = make(DataBagGroup, len(dataBagGroup))
			for i, dataBag := range dataBagGroup {
				clone.DataBags[dataType][dataName][i] = dataBag
			}
		}
	}
	return clone
}

func (c *ConfigContainer) IsEmpty() bool {
	return len(c.DataBags) == 0
}

func (c *ConfigContainer) DeleteDataBagsOfType(bagType string) {
	delete(c.DataBags, bagType)
}

func (c *ConfigContainer) DeleteDataBagGroup(bagType, bagName string) {
	if _, ok := c.DataBags[bagType]; ok {
		delete(c.DataBags[bagType], bagName)
	}
}

func (c *ConfigContainer) DeleteDataBag(bagType, bagName string, bagLabels []string) {
	if _, ok := c.DataBags[bagType]; ok {
		if _, ok := c.DataBags[bagType][bagName]; ok {
			for i, bag := range c.DataBags[bagType][bagName] {
				if reflect.DeepEqual(bag.Labels, bagLabels) {
					c.DataBags[bagType][bagName] = append(c.DataBags[bagType][bagName][:i], c.DataBags[bagType][bagName][i+1:]...)
					if len(c.DataBags[bagType][bagName]) == 0 {
						delete(c.DataBags[bagType], bagName)
					}
					if len(c.DataBags[bagType]) == 0 {
						delete(c.DataBags, bagType)
					}
					return
				}
			}
		}
	}
}

func (c *ConfigContainer) GetDataBagsOfType(bagType string) []DataBag {
	if m, ok := c.DataBags[bagType]; ok {
		arr := make([]DataBag, 0, len(m))
		for _, v := range m {
			arr = append(arr, v...)
		}
		return arr
	}
	return []DataBag{}
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
	c.DataBags[bag.Type][bag.Name], err = c.GetDataBagGroup(bag.Type, bag.Name).MergeWith([]DataBag{bag})
	return err
}

//Contains this only matches on the type/name/labels, not the content of the databag
func (c *ConfigContainer) Contains(bag DataBag) bool {
	if m, ok := c.DataBags[bag.Type]; ok {
		if group, ok := m[bag.Name]; ok {
			for _, v := range group {
				if reflect.DeepEqual(v.Labels, bag.Labels) {
					return true
				}
			}
		}
	}
	return false
}

func (c *ConfigContainer) MergeWith(other ConfigContainer) error {
	for dataType, dataBags := range other.DataBags {
		for dataName, dataBagGroup := range dataBags {
			var err error
			if _, ok := c.DataBags[dataType]; !ok {
				c.DataBags[dataType] = make(map[string]DataBagGroup)
			}
			if _, ok := c.DataBags[dataType][dataName]; !ok {
				c.DataBags[dataType][dataName] = make(DataBagGroup, 0, 1)
			}
			c.DataBags[dataType][dataName], err = c.GetDataBagGroup(dataType, dataName).MergeWith(dataBagGroup)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type ConcurrentConfigContainer struct {
	container *ConfigContainer
	lock      sync.RWMutex
}

func NewConcurrentConfigContainer() *ConcurrentConfigContainer {
	return &ConcurrentConfigContainer{
		container: NewConfigContainer(),
		lock:      sync.RWMutex{},
	}
}

func (c *ConcurrentConfigContainer) Insert(bag DataBag) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.container.Insert(bag)
}

func (c *ConcurrentConfigContainer) MergeWith(other ConfigContainer) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.container.MergeWith(other)
}

func (c *ConcurrentConfigContainer) Container() *ConfigContainer {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.container.Clone()
}

type Parser interface {
	Name() string
	CanParse(ctx context.Context, fileDesc fetcher.FileDescription) (bool, error)
	// Parse parses the file and returns only the part of the data it understands
	Parse(ctx context.Context, fileDesc fetcher.FileDescription, container *ConfigContainer) error
}

type TemplateEngine interface {
	Name() string
	//Apply cannot edit the input container, it must return a new one with the changes
	Apply(ctx context.Context, maker *Maker, input ConfigContainer, template fetcher.FileDescription) (ConfigContainer, error)
}

type Transformer interface {
	Name() string
	//Transform cannot edit the input container, and it must return a new config container with the new bags to create/modify
	Transform(ctx context.Context, container ConfigContainer) (ConfigContainer, error)
}

type Formatter interface {
	Name() string
	// Format formats the data from the parsed data in common format
	Format(ctx context.Context, container ConfigContainer) error
}
