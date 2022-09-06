# Barbe's library for Jsonnet templates

Article is the reference documentation for the Jsonnet `barbe` library available using `std.extVar("barbe")`

Find the source [here](https://github.com/Plenituz/barbe/blob/main/core/jsonnet_templater/barbe/utils.jsonnet)

Quick access:
- [regexFindAllSubmatch(pattern, input)](#regexFindAllSubmatchpattern-input)
- [flatten(arr)](#flattenarr)
- [databags(arr)](#databagsarr)
- [accumulateTokens(root, visitor)](#accumulateTokensroot-visitor)
- [visitTokens(root, visitor)](#visitTokensroot-visitor)
- [lookupTraverse(rootInput, traverse, errorPrefix)](#lookupTraverserootInput-traverse-errorPrefix)
- [lookupTraversal(root, traverseArr, errorPrefix)](#lookupTraversalroot-traverseArr-errorPrefix)
- [asStr(token)](#asStrtoken)
- [mergeTokens(values)](#mergeTokensvalues)
- [asVal(token)](#asValtoken)
- [asValArrayConst(token)](#asValArrayConsttoken)
- [asSyntax(val)](#asSyntaxval)
- [asTraversal(str)](#asTraversalstr)
- [appendToTraversal(source, toAdd)](#appendToTraversalsource-toAdd)
- [asFuncCall(funcName, args)](#asFuncCallfuncName-args)
- [asTemplate(arr)](#asTemplatearr)
- [asTemplateStr(arr)](#asTemplateStrarr)
- [concatStrArr(token)](#concatStrArrtoken)
- [appendToTemplate(source, toAdd)](#appendToTemplatesource-toAdd)
- [asBlock(arr)](#asBlockarr)
- [removeLabels(obj)](#removeLabelsobj)
- [iterateAllBlocks(container, func)](#iterateAllBlockscontainer-func)
- [iterateBlocks(container, ofType, func)](#iterateBlockscontainer-ofType-func)
- [compileDefaults(container, name)](#compileDefaultscontainer-name)
- [makeBlockDefault(container, globalDefaults, block)](#makeBlockDefaultcontainer-globalDefaults-block)

## regexFindAllSubmatch(pattern, input)

This function is a wrapper around Go's `regexp.FindAllStringSubmatch` function.

Arguments:
- `pattern`: `string`: The regex pattern to match
- `input`: `string`: The string to match against

Returns: `[][]string` The list of matches, each match being a list of submatches


```jsonnet
barbe.regexFindAllSubmatch("(.*)/(.*)/(.*)", "foo/bar/baz");
```

## flatten(arr)

Flattens an array of arrays or values into a single array, this is a wrapper over the `std.flatten` function which doesn't allows regular values.

Arguments:
- `arr`: `[]any`: The array to flatten

Returns: `[]any` The flattened array

```jsonnet
barbe.flatten([
    [1, 2, 3],
    8,
    [4, 5, 6],
    7,
])
// [1, 2, 3, 8, 4, 5, 6, 7]
```

## databags(arr)

This is a helper function that just wraps in the input array into an object with the `Databags` key.

Arguments:
- `arr`: `[]Databag` The array to wrap

Returns: `object` The wrapped array

```jsonnet
barbe.databags([
    {foo: "bar"},
    {foo: "baz"},
])
// {Databags: [{foo: "bar"}, {foo: "baz"}]}
```

## accumulateTokens(root, visitor)

This functions visits all the tokens in the input `root` object and calls the `visitor` function on each token, if the visitor returns `true` the token is added to the output array.

Arguments:
- `root`: `SyntaxToken`: The root object to visit
- `visitor`: `func`: The visitor function

Returns: `[]SyntaxToken` The list of tokens that the visitor returned `true` for

```jsonnet
barbe.accumulateTokens(
    bag.Value,
    function(token) token.Type == "literal_value"
)
```

## visitTokens(root, visitor)

This functions visits all the tokens in the input `root` object and calls the `visitor` function on each token. The visitor function can return a new token to replace the current one, or return `false` to let the visit continue.

Arguments:
- `root`: `SyntaxToken`: The root object to visit
- `visitor`: `func`: The visitor function

Returns: `any` The root object with the tokens replaced by the visitor

```jsonnet
barbe.visitTokens(
    bag.Value,
    function(token) 
        if token.Type == "literal_value" then
            {
                Type: "literal_value",
                Meta: std.get(token, "Meta", null),
                Value: std.strReplace(token.Value, "slur", "xxxx"),
            }
        else
            false
)
```

## lookupTraverse(rootInput, traverse, errorPrefix)

This function looks up a value in the input `rootInput` object using the `traverse` as a path.

You're probably looking for `lookupTraversal` instead.

Arguments:
- `rootInput`: `any`: The root object to lookup in
- `traverse`: [`Traverse`](#traverse): The path to lookup
- `errorPrefix`: `string`: The prefix to use in the error message if the lookup fails

Returns: `any` The value found at the path

```jsonnet
barbe.lookupTraverse(
    {foo: "bar"},
    {Type: "attr", Name: "foo"},
    ""
)
// "bar"
```

## lookupTraversal(root, traverseArr, errorPrefix)

This function looks up a value in the input `root` object using the `traverseArr` as a path.

Arguments:
- `root`: `any`: The root object to lookup in
- `traverseArr`: [`[]Traverse`](#traverse): The path to lookup
- `errorPrefix`: `string`: The prefix to use in the error message if the lookup fails

Returns: `any` The value found at the path

```jsonnet
barbe.lookupTraversal(
    {foo: {bar: "baz"}},
    [{Type: "attr", Name: "foo"}, {Type: "attr", Name: "bar"}],
    ""
)
// "baz"
```

## asStr(token)

This function converts a token into a string, or the string representation of the syntax token.

Arguments:
- `token`: `SyntaxToken`: The token to convert

Returns: `string` The string representation of the token

```jsonnet
barbe.asStr({Type: "literal_value", Value: "foo"})
// "foo"
```

## mergeTokens(values)

This function merges a list of tokens into a single token. It follows a few rules:
- The input tokens have to all be of the same type
- Arrays are merged into a single array
- Objects are merged into a single object
- For literal values, the last value is used

Arguments:
- `values`: `[]SyntaxToken`: The list of tokens to merge

Returns: `SyntaxToken` The merged token

```jsonnet
barbe.mergeTokens([
    {Type: "literal_value", Value: "foo"},
    {Type: "literal_value", Value: "bar"},
])
// {Type: "literal_value", Value: "bar"}

barbe.mergeTokens([
    {
        Type: "object_const",
        ObjectConst: [
           {Key: "foo", Value: {Type: "literal_value", Value: "bar"}},
        ],
    },
    {
        Type: "object_const",
        ObjectConst: [
           {Key: "baz", Value: {Type: "literal_value", Value: "qux"}},
        ],
    },    
])
// {
//     Type: "object_const",
//     ObjectConst: [
//        {Key: "foo", Value: {Type: "literal_value", Value: "bar"}},
//        {Key: "baz", Value: {Type: "literal_value", Value: "qux"}},
//     ],
// }
```

## asVal(token)

This function converts a token into a jsonnet value, when possible.

Arguments:
- `token`: `SyntaxToken`: The token to convert

Returns: `any` The jsonnet value

```jsonnet
barbe.asVal({Type: "literal_value", Value: "foo"})
// "foo"

barbe.asVal({
    Type: "object_const",
    ObjectConst: [
        {Key: "foo", Value: {Type: "literal_value", Value: "bar"}},
    ],
})
// {foo: "bar"}
```

## asValArrayConst(token)

This function converts an array_const token into an array of jsonnet value.

Arguments:
- `token`: `SyntaxToken`: The array_const token to convert

Returns: `[]any` The array of jsonnet values

```jsonnet
barbe.asValArrayConst({
    Type: "array_const",
    ArrayConst: [
        {Type: "literal_value", Value: "foo"},
        {Type: "literal_value", Value: "bar"},
    ],
})
// ["foo", "bar"]
```

## asSyntax(val)

This function converts a jsonnet value into a syntax token.

Arguments:
- `token`: `any`: The jsonnet value to convert

Returns: `SyntaxToken` The syntax token

```jsonnet
barbe.asSyntax("foo")
// {Type: "literal_value", Value: "foo"}
```

## asTraversal(str)

This function converts a dot separated string into a traversal syntax token.

Arguments:
- `str`: `string`: The string to convert

Returns: `SyntaxToken` The traversal syntax token

```jsonnet
barbe.asTraversal("foo.bar.baz")
// {
//     Type: "traversal",
//     Traversal: [
//         {Type: "attr", Name: "foo"},
//         {Type: "attr", Name: "bar"},
//         {Type: "attr", Name: "baz"},
//     ],
// }
```

## appendToTraversal(source, toAdd)

This function appends syntax tokens to a traversal syntax token.

Arguments:
- `source`: `SyntaxToken`: The traversal syntax token to append to
- `toAdd`: `[]SyntaxToken`: The syntax tokens to append

Returns: `SyntaxToken` The new traversal syntax token

```jsonnet
barbe.appendToTraversal(
    barbe.asTraversal("foo.bar"),
    [{Type: "attr", Name: "baz"}]
)
// {
//     Type: "traversal",
//     Traversal: [
//         {Type: "attr", Name: "foo"},
//         {Type: "attr", Name: "bar"},
//         {Type: "attr", Name: "baz"},
//     ],
// }
```

## asFuncCall(funcName, args)

This function converts a function name and a list of arguments into a function call syntax token.

Arguments:
- `funcName`: `string`: The name of the function to call
- `args`: `[](SyntaxToken | any)`: The arguments to pass to the function

Returns: `SyntaxToken` The function call syntax token

```jsonnet
barbe.asFuncCall("foo", ["bar"])
// {
//     Type: "func_call",
//     FunctionName: "foo",
//     FunctionArgs: [{Type: "literal_value", Value: "bar"}],
// }
```

## asTemplate(arr)

This function converts an array of syntax tokens or values into a template syntax token.

Arguments:
- `arr`: `[](SyntaxToken | any)`: The array to convert

Returns: `SyntaxToken` The template syntax token

```jsonnet
barbe.asTemplate(["foo", "bar"])
// {
//     Type: "template",
//     Parts: [
//         {Type: "literal_value", Value: "foo"},
//         {Type: "literal_value", Value: "bar"},
//     ],
// }
```

## asTemplateStr(arr)

Same as `asStr` except instead of converting the token into a string, it converts it into a template syntax token.

Arguments:
- `arr`: `[]SyntaxToken`: The array to convert

Returns: `SyntaxToken` The template syntax token

```jsonnet
barbe.asTemplateStr([
    {Type: "scope_traversal", Traversal: [{Type: "attr", Name: "foo"}]},
    {Type: "literal_value", Value: "bar"},
])
// {
//     Type: "template",
//     Parts: [
//         {Type: "scope_traversal", Traversal: [{Type: "attr", Name: "foo"}]},
//         {Type: "literal_value", Value: "bar"},
//     ],
// }
```

## concatStrArr(token)

This function concatenates an array_const token into a single template syntax token using `asTemplateStr`.

Arguments:
- `token`: `SyntaxToken`: The array to concatenate

Returns: `SyntaxToken` The template syntax token

```jsonnet
barbe.concatStrArr({
    Type: "array_const",
    ArrayConst: [
        {Type: "literal_value", Value: "foo"},
        {Type: "literal_value", Value: "bar"},
    ],
})
// {
//     Type: "template",
//     Parts: [
//         {Type: "literal_value", Value: "foo"},
//         {Type: "literal_value", Value: "bar"},
//     ],
// }
```

## appendToTemplate(source, toAdd)

This function appends syntax tokens to a template syntax token.

Arguments:
- `source`: `SyntaxToken`: The template syntax token to append to
- `toAdd`: `[]SyntaxToken`: The syntax tokens to append

Returns: `SyntaxToken` The new template syntax token

```jsonnet
barbe.appendToTemplate(
    barbe.asTemplate(["foo"]),
    [{Type: "literal_value", Value: "bar"}]
)
// {
//     Type: "template",
//     Parts: [
//         {Type: "literal_value", Value: "foo"},
//         {Type: "literal_value", Value: "bar"},
//     ],
// }
```

## asBlock(arr)

This function converts an array of object_const syntax tokens or values into an array_const syntax tokens tagged with `IsBlock: true` in the `Meta` field.

Arguments:
- `arr`: `[](SyntaxToken | any)`: The array to convert

Returns: `SyntaxToken` The array_const syntax token

```jsonnet
barbe.asBlock([
    {Type: "object_const", ObjectConst: [{Key: "foo", Value: "bar"}]},
    {Type: "object_const", ObjectConst: [{Key: "baz", Value: "qux"}]},
])
// {
//     Type: "array_const",
//     ArrayConst: [
//         {Type: "object_const", Meta: {IsBlock: true}, ObjectConst: [{Key: "foo", Value: "bar"}]},
//         {Type: "object_const", Meta: {IsBlock: true}, ObjectConst: [{Key: "baz", Value: "qux"}]},
//     ],
//     Meta: {IsBlock: true}
// }
```

## removeLabels(obj)

This function removes all fields named `labels` from an object_const syntax token. This is useful for HCL blocks that have labels but are not top level blocks.

Arguments:
- `obj`: `SyntaxToken`: The object_const syntax token to remove labels from

Returns: `SyntaxToken` The object_const syntax token without labels

```jsonnet
barbe.removeLabels({
    Type: "object_const",
    ObjectConst: [
        {Key: "foo", Value: "bar"},
        {Key: "labels", Value: {Type: "array_const", ArrayConst: []}},
    ],
})
// {
//     Type: "object_const",
//     ObjectConst: [
//         {Key: "foo", Value: "bar"},
//     ],
// }
```

## iterateAllBlocks(container, func)

This function iterates over all databags in a databag container and calls a function for each databag.

Arguments:
- `container`: `DatabagContainer`: The databag container to iterate over
- `func`: `func(databag)`: The function to call for each databag

Returns: `[]any` The results of the function calls

```jsonnet
barbe.iterateAllBlocks(
    container,
    function(databag) databag.Type
)
```

## iterateBlocks(container, ofType, func)

This function iterates over all databags of a specific type in a databag container and calls a function for each databag.

Arguments:
- `container`: `DatabagContainer`: The databag container to iterate over
- `ofType`: `string`: The type of databag to iterate over
- `func`: `func(databag)`: The function to call for each databag

Returns: `[]any` The results of the function calls

```jsonnet
barbe.iterateBlocks(
    container,
    "resource",
    function(databag) databag.Name
)
```

## compileDefaults(container, name)

This function is meant for Barbe-serverless default blocks, it will eventually be moved out of the core library.

## makeBlockDefault(container, globalDefaults, block)

This function is meant for Barbe-serverless default blocks, it will eventually be moved out of the core library.

---

# Common types

## Traverse
    
```cue
{
	Type: "attr"
	Name: string
} | {
	Type: "index"
    Index: int | string
}
```