# Understanding barbe components

Components are the main characters in barbe, they contain all the logic that transforms your configuration files into infrastructure and command executions.

This article is more about the concept of barbe components, rather than an how-to guide on creating your own.

Each component receives the syntax tokens representing the input configuration, and can return new syntax tokens that will eventually be turned into terraform files, command executions, or anything else. Component should be idempotent, meaning they will always return the same output for the same input, because they get executed several times and in parallel to other components.

Let's take an example of a single component being executed:

- `aws_function.jsonnet` gets executed with input
```hlc
aws_function "my-function" {...}
```
- `aws_function.jsonnet` runs and creates new syntax tokens that get added to the same "pool" of syntax tokens, you can imagine it creating a log group and a lambda function. The pool of token now contains
```hlc
aws_function "my-function" {...}
resource "aws_log_group" "my-function-log-group" {...}
resource "aws_lambda_function" "my-function" {...}
```
- Since the component created new syntax tokens, it gets executed again with the new pool of tokens. As expected it creates the same resources again. Barbe compares all the newly created resources with the existing ones and realizes that there is no new change, so it stops executing the component here.

When you introduce several components, the behavior is very similar:
- All the components get executed in parallel with the input tokens
- All the newly generated tokens get added to the pool of tokens
- The new token pool gets passed to all the components again
- Loop that until all the components stop creating new tokens

### Why tho?

This seems like a lot of extra compute for something fairly simple. Why not just execute the component once and be done with it?

This is done to be able to keep the component implementations completely isolated from each other. 
Imagine a component that transforms any reference in the input config like `env.MY_VAR` into the actual value of the environment variable `MY_VAR`. If this component was executed just once, only the references to `env.MY_VAR` in the original configuration file would be replaced, so if another component, like `aws_function.jsonnet`, were to generate a lambda function with a reference to `env.MY_VAR`, it would not be replaced.