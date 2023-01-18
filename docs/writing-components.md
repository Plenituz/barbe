# Writing templates

Templates are currently written in [jsonnet](https://jsonnet.org/), it's a data templating language that's fairly easy
to get started with, it's syntax has some similarities with python

The sandboxed nature of it is an important factor for Barbe, as we are executing code that could be tempered with. 
Barbe is however not restricted to one templating language, and may see other options appear in the future

### How barbe works under the hood

Before we start writing templates, let's get a refresher on the internals of Barbe.

Barbe is a syntax manipulation tool, this means when you write Barbe templates, you are manipulating syntax tokens. 
A syntax token is a data structure that represents a piece of syntax of a configuration file, for example a number or string would be represented as a "literal_value" token:
```cue
// this is the syntax token for `42`
{
    Type: "literal_value",
    Value: 42
}
```

This might not seem very useful on such a simple value, so let's take a more complex example, a reference in HCL:
```hcl
bucket_name = aws_s3_bucket.my_bucket.id
```

Internally this kind of syntax is called a "traversal", here is its syntax token representation:
```cue
{
    Type: "scope_traversal",
    Traversal: [
    	{
    		Type: "attr",
    		Name: "aws_s3_bucket",
    	},
    	{
    		Type: "attr",
    		Name: "my_bucket",
    	},
    	{
    		Type: "attr",
    		Name: "id",
    	}
    ]
}
```

Using this kind of data structure it becomes very easy to manipulate syntax. Say you want to map the name `my_bucket` to
an internal name, it is trivial to iterate an array and if `Name == "my_bucket"` replace it with `Name = "internal_name"`

If you want more details on all the different syntax tokens, take a look at the [syntax tokens](./syntax-tokens.md) page.

For now let's take a quick look at 3 more that you will most commonly encounter.


#### Object const

The `object_const` token represents an object, for example this object
```cue
{
	abc: "def",
	hij: 42
}
```

Would be represented as
```cue
{
    Type: "object_const",
    ObjectConst: [
        {
            Key: "abc",
            Value: {
                Type: "literal_value",
                Value: "def"
            }
        },
        {
            Key: "hij",
            Value: {
                Type: "literal_value",
                Value: 42
            }
        }
    ]
}
```

#### Array const

The `array_const` token represents an array, for example this array
```cue
["abc", 42]
```

Would be represented as
```cue
{
    Type: "array_const",
    ArrayConst: [
        {
            Type: "literal_value",
            Value: "abc"
        },
        {
            Type: "literal_value",
            Value: 42
        }
    ]
}
```

#### String template

The `template` token represents a string interpolation template, for example this template
```cue
"prefix ${42} middle ${aws_s3_bucket.my_bucket.id}"
```

Would be represented as
```cue
{
    Type: "template",
    Parts: [
        {
            Type: "literal_value",
            Value: "prefix "
        },
        {
            Type: "literal_value",
            Value: 42
        },
        {
            Type: "literal_value",
            Value: " middle "
        },
        {
            Type: "scope_traversal",
            Traversal: [
                {
                    Type: "attr",
                    Name: "aws_s3_bucket",
                },
                {
                    Type: "attr",
                    Name: "my_bucket",
                },
                {
                    Type: "attr",
                    Name: "id",
                }
            ]
        }
    ]
}
```


### The input/output of a template

The syntax tokens your template receives as input are grouped together in `databags`. A databag is just a syntax token that has an extra arbitrary `Type` and `Name`.
Note that the `Type` of a databag is completely different from the `Type` of the syntax token it contains. 

The databag's type and name are just strings that identify it amongst all the other databags, it can come from the user's configuration file, or from another template that generated the databag.
For example, this is what the databag of a `aws_function` block could look like from Barbe-serverless

The hcl block being parsed
```hcl
aws_function "my-func-name" {
  handler = "my-func.handler"
  runtime = "python3.8"
}
```

The produced `databag`
```cue
{
	Type: "aws_function",
	Name: "my-func-name",
	Value: {
		Type: "object_const",
        ObjectConst: [
            {
                Key: "handler",
                Value: {
                    Type: "literal_value",
                    Value: "my-func.handler"
                }
            },
            {
                Key: "runtime",
                Value: {
                    Type: "literal_value",
                    Value: "python3.8"
                }
            }
        ]
	}
}
```

The main input to your template will be a map of databags that is indexed by the databag's `Type` and then `Name`. 
So if we imagine the previous `aws_function` block being the only parsed input to your template, the input would be

```cue
{
    "aws_function": {
        "my-func-name": [
        	{
                Type: "aws_function",
                Name: "my-func-name",
                Value: {
                    Type: "object_const",
                    ObjectConst: [
                        {
                            Key: "handler",
                            Value: {
                                Type: "literal_value",
                                Value: "my-func.handler"
                            }
                        },
                        {
                            Key: "runtime",
                            Value: {
                                Type: "literal_value",
                                Value: "python3.8"
                            }
                        }
                    ]
                }
            }
        ]
    }
}
```

If another block with the exact same `Type` and `Name` was parsed, it would be added to the array under `aws_function.my-func-name`. 
If another block with the same `Type` but a different `Name` was parsed, it would be added to the map under `aws_function` with a different key.

This indexing is done to make it efficient to iterate blocks with the same type, or to access a specific databag if you already know it's type/name combination.

```go
// pseudo code for iterating all databags of type "aws_function"
for aws_function in input.aws_function {
    for databagName, databagList in aws_function {
        for databag in databagList {
			// do something with databag
        }
    }
}

// pseudo code for accessing a specific databag
databag = input.aws_function.my-func-name[0]
```


### Your first Jsonnet template

#### Basic structure and running a template

Barbe templates can be written in [Jsonnet](https://jsonnet.org/). 
I suggest taking a look at the [Jsonnet tutorial](https://jsonnet.org/learning/tutorial.html) before you get started with Barbe templates, 
just make sure you have enough understanding to be able to read the examples. 

Let's start by creating a new template file `my_template.jsonnet` in a directory

```jsonnet
# my_template.jsonnet
{
    Databags: [
        {
            Type: "raw_file",
            Name: "my_raw_file",
            Value: {
                path: "my_raw_file.txt",
                content: "Hello world!",
            }
        }
    ]
}
```

The snippet above highlights the basic structure of a Barbe template:
- You need the output of the template to be a map with a `Databags` key
- The value of the `Databags` key must be an array of databags

You can think of each databag as an instruction to some formatter. 
The formatters are the processes that run inside Barbe after your templates. 
They are the ones that actually create files, run commands, etc, based on the databags that templates create.

In this example, we are telling the `raw_file` formatter, whose role is to create text files, to create a file named `my_raw_file.txt` with the content `Hello world!`. 
You can see a list of the current formatters on the [formatters](./formatters.md) page.

For now, let's run this example template to see our file being created. 
We'll need a `config.hcl` for that
```hcl
# config.hcl
template {
  template = "./my_template.jsonnet"
}
```

Then we can run
```bash
barbe generate config.hcl
```

Our file should be created in the `dist` directory
```
# dist
# └── my_raw_file.txt

cat barbe_dist/my_raw_file.txt
# Hello world!
```

#### Using the input

Our previous example generated a file but it didn't use any input. 
Our Barbe template has access to all the databags that were the results of parsing the input files,and the templates that were executed before ours, if any. 
In our case that's just the content of the `config.hcl` file we created.

To access the input databags we use the `container` extVar in Jsonnet.

```jsonnet
# my_template.jsonnet
local container = std.extVar("container");
{
    Databags: [
        {
            Type: "raw_file",
            Name: "my_raw_file",
            Value: {
                path: "my_raw_file.json",
                content: container + "",
            }
        }
    ]
}
```

This updated template will create a file named `my_raw_file.json` with the content of the `container` extVar. 
This allows us to see what we're getting as an input, running `barbe generate` again will yield this json file

```json
# barbe_dist/my_raw_file.json
{
    "template": {
        "": [
            {
                "Labels": [],
                "Name": "",
                "Type": "template",
                "Value": {
                    "Type": "object_const"
                    "Meta": { "IsBlock": true },
                    "ObjectConst": [
                        {
                            "Key": "template",
                            "Value": {
                                "Type": "literal_value",
                                "Value": "./my_template.jsonnet"
                            }
                        }
                    ]
                }
            }
        ]
    }
}
```

As you can see, the `container` is a map of databags indexed by `Type` and `Name`, as we saw in the previous section.
And currently it only has the `template` block that we created in the `config.hcl` file. Let's change that

```hcl
# config.hcl
template {
  template = "./my_template.jsonnet"
}

user "bob" {
  job = "developer"
}
```

```jsonnet
# my_template.jsonnet
local container = std.extVar("container");
{
    Databags: [
        {
            Type: "raw_file",
            Name: "bob",
            Value: {
                path: "bob.json",
                content: "" + {
                    username: "bob",
                    job: container.user.bob[0].Value.ObjectConst[0].Value.Value,
                },
            }
        }
    ]
}
```

```json
# barbe_dist/bob.json
{"job": "developer", "username": "bob"}
```

As you can see we can access the values from the `user` databag by using the `container` extVar.
The `job` field is very verbose and makes a lot of assumption on what the input databag looks like, let's break it down

```cue
container.user.bob[0].Value.ObjectConst[0].Value.Value // => "developer"
container.user.bob[0].Value.ObjectConst[0].Value // => {"Type": "literal_value", "Value": "developer"}
container.user.bob[0].Value.ObjectConst[0] // => {"Key": "job", "Value": {"Type": "literal_value", "Value": "developer"}}
container.user.bob[0].Value // => {"Type": "object_const", "ObjectConst": [...]}
container.user.bob[0] // => {"Name": "bob", "Type": "user", "Value": {"Type": "object_const", "ObjectConst": [...]}}
```


#### Using the `barbe` library

To make manipulating syntax tokens easier, Barbe provides a `barbe` library that you can find under `std.extVar("barbe")`.
Jsonnet also provides a `std` library that provides a lot of utility functions.

Let's convert the previous section's example using the `barbe` library

```jsonnet
local container = std.extVar("container");
local barbe = std.extVar("barbe");
barbe.databags([
    barbe.iterateBlocks(container, "user", function(bag)
        {
            Type: "raw_file",
            Name: bag.Name,
            Value: {
                path: bag.Name + ".json",
                content: "" + {
                    local blockAsObject = barbe.asVal(bag.Value),

                    username: bag.Name,
                    job: barbe.asStr(blockAsObject.job),
                },
            }
        }
    )
])
```

Lets breakdown the functions we used:
- `barbe.databags` saves you and indentation level by wrapping the array of databags in a `Databags` key
- `barbe.iterateBlocks` is a helper function that iterates over all the databags of a given type, in our case `user` and invokes the callback function for each of them. This will make it so our template works for any number of `user` block in the input `config.hcl`.
- `barbe.asVal` converts a syntax token to a Jsonnet value. This is useful in a lot of cases, but it's also incompatible with a few syntax tokens. For example if the user has put a function call in his `config.hcl`, calling `barbe.asVal` on that would not work. So make sure to not `barbe.asVal` on a syntax token that you don't know the type of.
- `barbe.asStr` converts a syntax token to a string. It's slightly different from `barbe.asVal` as it will do it's best to find a string representation for the given syntax token. For example `reference.to.foo` would be converted to `"reference.to.foo"`.

With our modified template we can now add as many `user` block in our `config.hcl` as we want and see the `dist` folder get populated with json files.

```hcl
# config.hcl
template {
  template = "./my_template.jsonnet"
}

user "bob" {
  job = "developer"
}
user "emily" {
  job = "developer"
}
user "john" {
  job = "marketeer"
}
```

```
dist
├── bob.json
├── emily.json
└── john.json
```

For more details on the barbe library you can check out the [Barbe std](./barbe-std.md) page.
You can also check out the [Barbe-serverless](https://github.com/Plenituz/barbe-serverless) repo to see how the templates are made

### Tips on debugging/developing templates

- Use `std.trace` to print out values in your template
```cue
value: std.trace(thing.Value+"", thing.Value)
```

- Use `assert` and `error` statements for data validation if needed
```jsonnet
if !std.objectHas(thing, "foo") then
    error "foo is missing"
else 
    thing.foo;
# or
assert std.objectHas(thing, "foo") : "foo is missing"
```

- Unreferenced variables do not get evaluated in Jsonnet, this is why `std.trace` needs 2 values
```jsonnet
# if not referenced, this will not get evaluated, so nothing will be printed
local debug = std.trace("debug", "debug");
#instead do something like
{
    Value: std.trace("debug", thing.Value)
}
```
