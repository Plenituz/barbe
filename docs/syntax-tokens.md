# Syntax tokens

This article lists all the currently supported syntax tokens, what their data structure looks like and what their equivalent in HCL is.

In all the data structure pseudo types below `SyntaxToken` means any syntax token data structure could go there.

Find the source [here](https://github.com/Plenituz/barbe/blob/main/core/common_format.go)

Quick access:
- [Literal value](#literal-value)
- [Scope traversal](#scope-traversal)
- [Function call](#function-call)
- [Template](#template)
- [Object const](#object-const)
- [Array const](#array-const)
- [Index access](#index-access)
- [For](#for)
- [Relative traversal](#relative-traversal)
- [Conditional](#conditional)
- [Binary op](#binary-op)
- [Unary op](#unary-op)
- [Parens](#parens)

## Literal value

Data structure pseudo-type:
```cue
{
    Type: "literal_value"
    Value: boolean | number | string
}
```

HCL examples:
```hcl
1
#{
#  Type: "literal_value"
#  Value: 1
#}

"hello"
#{
#  Type: "literal_value"
#  Value: "hello"
#}

true
#{
#  Type: "literal_value"
#  Value: true
#}
```

Notes:
- The HCL parser will almost always use `template` tokens for string literals, so this token is mostly used for numbers and booleans.

## Scope traversal

Data structure pseudo-type:
```cue
{
    Type: "scope_traversal"
    Traversal: [...Traverse]
}
Traverse = {
	Type: "attr"
	Name: string
} | {
	Type: "index"
    Index: int | string
}
```

HCL examples:
```hcl
foo.bar
#{
#  Type: "scope_traversal"
#  Traversal: [
#    {
#      Type: "attr"
#      Name: "foo"
#    },
#    {
#      Type: "attr"
#      Name: "bar"
#    }
#  ]
#}

foo[0]
#{
#  Type: "scope_traversal"
#  Traversal: [
#    {
#      Type: "attr"
#      Name: "foo"
#    },
#    {
#      Type: "index"
#      Index: 0
#    }
#  ]
#}
```

## Function call

Data structure pseudo-type:
```cue
{
    Type: "function_call"
    FunctionName: string
    FunctionArgs: [...SyntaxToken]
}
```

HCL examples:
```hcl
foo()
#{
#  Type: "function_call"
#  FunctionName: "foo"
#  FunctionArgs: []
#}

foo(1, 2, 3)
#{
#  Type: "function_call"
#  FunctionName: "foo"
#  FunctionArgs: [
#    {
#      Type: "literal_value"
#      Value: 1
#    },
#    {
#      Type: "literal_value"
#      Value: 2
#    },
#    {
#      Type: "literal_value"
#      Value: 3
#    }
#  ]
#}
```

## Template

Data structure pseudo-type:
```cue
{
    Type: "template"
    Parts: [...SyntaxToken]
}
```

HCL examples:
```hcl
"hello ${foo}"
#{
#  Type: "template"
#  Parts: [
#    {
#      Type: "literal_value"
#      Value: "hello "
#    },
#    {
#      Type: "scope_traversal"
#      Traversal: [
#        {
#          Type: "attr"
#          Name: "foo"
#        }
#      ]
#    }
#  ]
#}
```

## Object const

Data structure pseudo-type:
```cue
{
    Type: "object_const"
    ObjectConst: [...ObjectConstItem]
}
ObjectConstItem = {
    Key: string
    Value: SyntaxToken
}
```

HCL examples:
```hcl
{
    foo = 1
    bar = 2
}
#{
#  Type: "object_const"
#  ObjectConst: [
#    {
#      Key: "foo"
#      Value: {
#        Type: "literal_value"
#        Value: 1
#      }
#    },
#    {
#      Key: "bar"
#      Value: {
#        Type: "literal_value"
#        Value: 2
#      }
#    }
#  ]
#}
```

## Array const

Data structure pseudo-type:
```cue
{
    Type: "array_const"
    ArrayConst: [...SyntaxToken]
}
```

HCL examples:
```hcl
[1, 2, 3]
#{
#  Type: "array_const"
#  ArrayConst: [
#    {
#      Type: "literal_value"
#      Value: 1
#    },
#    {
#      Type: "literal_value"
#      Value: 2
#    },
#    {
#      Type: "literal_value"
#      Value: 3
#    }
#  ]
#}
```

## Index access

Data structure pseudo-type:
```cue
{
    Type: "index_access"
    IndexCollection: SyntaxToken
    IndexKey: SyntaxToken
}
```

HCL examples:
```hcl
foo[0]
#{
#  Type: "index_access"
#  IndexCollection: {
#    Type: "scope_traversal"
#    Traversal: [
#      {
#        Type: "attr"
#        Name: "foo"
#      }
#    ]
#  }
#  IndexKey: {
#    Type: "literal_value"
#    Value: 0
#  }
#}

foo["bar"]
#{
#  Type: "index_access"
#  IndexCollection: {
#    Type: "scope_traversal"
#    Traversal: [
#      {
#        Type: "attr"
#        Name: "foo"
#      }
#    ]
#  }
#  IndexKey: {
#    Type: "literal_value"
#    Value: "bar"
#  }
#}
```

Note:
- This token is rarely used, relative traversals are usually used instead.

## For

Data structure pseudo-type:
```cue
{
    Type: "for"
    ForKeyVar: string | null
    ForValVar: string
    ForCollExpr: SyntaxToken
    ForKeyExpr: SyntaxToken | null
    ForValExpr: SyntaxToken
    ForCondExpr: SyntaxToken | null
}
```

HCL examples:
```hcl
[for i, v in list: v if i > 2]
#{
#  "Type": "for",
#  "ForValVar": "v",
#  "ForKeyVar": "i",
#  "ForCollExpr": {
#    "Traversal": [{ "Name": "list", "Type": "attr" }],
#    "Type": "scope_traversal"
#  },
#  "ForValExpr": {
#    "Traversal": [{ "Name": "v", "Type": "attr" }],
#    "Type": "scope_traversal"
#  },
#  "ForCondExpr": {
#    "LeftHandSide": {
#      "Traversal": [{ "Name": "i", "Type": "attr" }],
#      "Type": "scope_traversal"
#    },
#    "Operator": ">",
#    "RightHandSide": { "Type": "literal_value", "Value": 2 },
#    "Type": "binary_op"
#  }
#}

{for k, v in map: k => v}
#{
#  "Type": "for",
#  "ForValVar": "v",
#  "ForKeyVar": "k",
#  "ForCollExpr": {
#    "Traversal": [{ "Name": "map", "Type": "attr" }],
#    "Type": "scope_traversal"
#  },
#  "ForKeyExpr": {
#    "Traversal": [{ "Name": "k", "Type": "attr" }],
#    "Type": "scope_traversal"
#  },
#  "ForValExpr": {
#    "Traversal": [{ "Name": "v", "Type": "attr" }],
#    "Type": "scope_traversal"
#  }
#}
```

## Relative traversal

Data structure pseudo-type:
```cue
{
    Type: "relative_traversal"
    Source: SyntaxToken
    Traversal: [...Traverse]
}
Traverse = {
	Type: "attr"
	Name: string
} | {
	Type: "index"
    Index: int | string
}
```

HCL examples:
```hcl
[1, 2][1]
#{
#  Type: "relative_traversal"
#  Source: {
#    Type: "array_const"
#    ArrayConst: [
#      {
#        Type: "literal_value"
#        Value: 1
#      },
#      {
#        Type: "literal_value"
#        Value: 2
#      }
#    ]
#  }
#  Traversal: [
#    {
#      Type: "index"
#      Index: 1
#    }
#  ]
#}

{hello = "world"}["hello"]
#{
#  Type: "relative_traversal"
#  Source: {
#    Type: "object_const"
#    ObjectConst: [
#      {
#        Key: "hello"
#        Value: {
#          Type: "literal_value"
#          Value: "world"
#        }
#      }
#    ]
#  }
#  Traversal: [
#    {
#      Type: "attr"
#      Index: "hello"
#    }
#  ]
#}
```

## Conditional

Data structure pseudo-type:
```cue
{
    Type: "conditional"
    Condition: SyntaxToken
    TrueResult: SyntaxToken
    FalseResult: SyntaxToken
}
```

HCL examples:
```hcl
true ? 1 : 2
#{
#  Type: "conditional"
#  Condition: {
#    Type: "literal_value"
#    Value: true
#  }
#  TrueResult: {
#    Type: "literal_value"
#    Value: 1
#  }
#  FalseResult: {
#    Type: "literal_value"
#    Value: 2
#  }
#}
```

## Binary op

Data structure pseudo-type:
```cue
{
    Type: "binary_op"
    LeftHandSide: SyntaxToken
    Operator: string
    RightHandSide: SyntaxToken
}
```

HCL examples:
```hcl
1 + 2
#{
#  Type: "binary_op"
#  LeftHandSide: {
#    Type: "literal_value"
#    Value: 1
#  }
#  Operator: "+"
#  RightHandSide: {
#    Type: "literal_value"
#    Value: 2
#  }
#}
```

## Unary op

Data structure pseudo-type:
```cue
{
    Type: "unary_op"
    Operator: string
    RightHandSide: SyntaxToken
}
```

HCL examples:
```hcl
!true
#{
#  Type: "unary_op"
#  Operator: "!"
#  RightHandSide: {
#    Type: "literal_value"
#    Value: true
#  }
#}
```

## Parens

Data structure pseudo-type:
```cue
{
    Type: "parens"
    Source: SyntaxToken
}
```

HCL examples:
```hcl
(1 + 2)
#{
#  Type: "parens"
#  Source: {
#    Type: "binary_op"
#    LeftHandSide: {
#      Type: "literal_value"
#      Value: 1
#    }
#    Operator: "+"
#    RightHandSide: {
#      Type: "literal_value"
#      Value: 2
#    }
#  }
#}
```