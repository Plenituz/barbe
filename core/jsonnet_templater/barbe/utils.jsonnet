local barbe = {
    regexFindAllSubmatch:: std.native("regexFindAllSubmatch"),

    flatten(arr)::
        if std.isArray(arr) && std.length(std.filter(std.isArray, arr)) == 0 then
            arr
        else
            std.flattenArrays([
                if std.isArray(item) then
                    barbe.flatten(item)
                else
                    [item]
                for item in arr
            ]),

    databags(arr):: { Databags: barbe.flatten(arr) },

    pipelines(pipes)::
        local command = std.extVar("barbe_command");
        local selectedPipeline = std.extVar("barbe_selected_pipeline");
        local selectedStep = std.extVar("barbe_selected_pipeline_step");
        if selectedPipeline == "" then
            {
                Pipelines: [
                    if std.objectHas(pipe, command) then
                        std.length(pipe[command])
                    else
                        0
                    for pipe in pipes
                ],
            }
         else
            local pipe = pipes[std.parseInt(selectedPipeline)];
            local step =
                if std.objectHas(pipe, command) then
                    pipe[command][std.parseInt(selectedStep)]
                else
                    function(_) barbe.databags([])
                ;
            {
                Pipelines: step(std.extVar("container")),
            }
        ,

    accumulateTokens(root, visitor)::
        local shouldKeep = visitor(root);
        local arr = barbe.flatten([
            if shouldKeep then
                root
            ,
            if root.Type == "anon" ||
                root.Type == "literal_value" ||
                root.Type == "scope_traversal" then
                null
            else if root.Type == "relative_traversal" then
                barbe.accumulateTokens(root.Source, visitor)
            else if root.Type == "splat" then
                [
                    barbe.accumulateTokens(root.Source, visitor),
                    barbe.accumulateTokens(root.SplatEach, visitor)
                ]
            else if root.Type == "object_const" then
                [
                    barbe.accumulateTokens(item.Value, visitor)
                    for item in std.get(root, "ObjectConst", [])
                ]
            else if root.Type == "array_const" then
                [
                    barbe.accumulateTokens(item, visitor)
                    for item in std.get(root, "ArrayConst", [])
                ]
            else if root.Type == "template" then
                [
                    barbe.accumulateTokens(item, visitor)
                    for item in root.Parts
                ]
            else if root.Type == "function_call" then
                [
                    barbe.accumulateTokens(item, visitor)
                    for item in root.FunctionArgs
                ]
            else if root.Type == "index_access" then
                [
                    barbe.accumulateTokens(root.IndexCollection, visitor),
                    barbe.accumulateTokens(root.IndexKey, visitor)
                ]
            else if root.Type == "conditional" then
                [
                    barbe.accumulateTokens(root.Condition, visitor),
                    barbe.accumulateTokens(root.TrueResult, visitor),
                    barbe.accumulateTokens(root.FalseResult, visitor)
                ]
            else if root.Type == "parens" then
                barbe.accumulateTokens(root.Source, visitor)
            else if root.Type == "binary_op" then
                [
                    barbe.accumulateTokens(root.RightHandSide, visitor),
                    barbe.accumulateTokens(root.LeftHandSide, visitor)
                ]
            else if root.Type == "unary_op" then
                barbe.accumulateTokens(root.RightHandSide, visitor)
            else if root.Type == "for" then
                [
                    barbe.accumulateTokens(root.ForCollExpr, visitor),
                    if std.objectHas(root, "ForKeyExpr") then barbe.accumulateTokens(root.ForKeyExpr, visitor) else null,
                    barbe.accumulateTokens(root.ForValExpr, visitor),
                    if std.objectHas(root, "ForCondExpr") then barbe.accumulateTokens(root.ForCondExpr, visitor) else null,
                ]
        ]);
        [i for i in arr if i != null]
        ,


    visitTokens(root, visitor)::
        local result = visitor(root);
        if std.isObject(result) then
            result
        else
            if root.Type == "anon" ||
                root.Type == "literal_value" ||
                root.Type == "scope_traversal" then
                root
            else if root.Type == "relative_traversal" then
                {
                    Type: "relative_traversal",
                    Source: barbe.visitTokens(root.Source, visitor),
                    Traversal: root.Traversal
                }
            else if root.Type == "splat" then
                {
                    Type: "splat",
                    Meta: std.get(root, "Meta", null),
                    Source: barbe.visitTokens(root.Source, visitor),
                    SplatEach: barbe.visitTokens(root.SplatEach, visitor),
                }
            else if root.Type == "object_const" then
                {
                    Type: "object_const",
                    Meta: std.get(root, "Meta", null),
                    ObjectConst: [
                        {
                            Key: item.Key,
                            Value: barbe.visitTokens(item.Value, visitor)
                        }
                        for item in std.get(root, "ObjectConst", [])
                    ],
                }
            else if root.Type == "array_const" then
                {
                    Type: "array_const",
                    Meta: std.get(root, "Meta", null),
                    ArrayConst: [
                        barbe.visitTokens(item, visitor)
                        for item in std.get(root, "ArrayConst", [])
                    ],
                }
            else if root.Type == "template" then
                {
                    Type: "template",
                    Meta: std.get(root, "Meta", null),
                    Parts: [
                        barbe.visitTokens(item, visitor)
                        for item in root.Parts
                    ],
                }
            else if root.Type == "function_call" then
                {
                    Type: "function_call",
                    Meta: std.get(root, "Meta", null),
                    FunctionName: root.FunctionName,
                    FunctionArgs: [
                        barbe.visitTokens(item, visitor)
                        for item in root.FunctionArgs
                    ],
                }
            else if root.Type == "index_access" then
                {
                    Type: "index_access",
                    Meta: std.get(root, "Meta", null),
                    IndexCollection: barbe.visitTokens(root.IndexCollection, visitor),
                    IndexKey: barbe.visitTokens(root.IndexKey, visitor),
                }
            else if root.Type == "conditional" then
                {
                    Type: "conditional",
                    Meta: std.get(root, "Meta", null),
                    Condition: barbe.visitTokens(root.Condition, visitor),
                    TrueResult: barbe.visitTokens(root.TrueResult, visitor),
                    FalseResult: barbe.visitTokens(root.FalseResult, visitor),
                }
            else if root.Type == "parens" then
                {
                    Type: "parens",
                    Meta: std.get(root, "Meta", null),
                    Source: barbe.visitTokens(root.Source, visitor),
                }
            else if root.Type == "binary_op" then
                {
                    Type: "binary_op",
                    Meta: std.get(root, "Meta", null),
                    Operator: root.Operator,
                    RightHandSide: barbe.visitTokens(root.RightHandSide, visitor),
                    LeftHandSide: barbe.visitTokens(root.LeftHandSide, visitor),
                }
            else if root.Type == "unary_op" then
                {
                    Type: "unary_op",
                    Meta: std.get(root, "Meta", null),
                    Operator: root.Operator,
                    RightHandSide: barbe.visitTokens(root.RightHandSide, visitor),
                }
            else if root.Type == "for" then
                {
                    Type: "for",
                    Meta: std.get(root, "Meta", null),
                    ForKeyVar: root.ForKeyVar,
                    ForValVar: root.ForValVar,
                    ForCollExpr: barbe.visitTokens(root.ForCollExpr, visitor),
                    ForKeyExpr: if std.objectHas(root, "ForKeyExpr") then barbe.visitTokens(root.ForKeyExpr, visitor) else null,
                    ForValExpr: barbe.visitTokens(root.ForValExpr, visitor),
                    ForCondExpr: if std.objectHas(root, "ForCondExpr") then barbe.visitTokens(root.ForCondExpr, visitor) else null,
                }
        ,

    // lookup the value of a single traverse on an object or array
    // root is a syntax token of type object_const or array_const
    // traverse is a Traverse object, typically from a scope_traversal.Traverse array
    lookupTraverse(rootInput, traverse, errorPrefix)::
        local root =
            if std.get(std.get(rootInput, "Meta", {}), "IsBlock", false) && std.length(std.get(rootInput, "ArrayConst", [])) == 1 then
                rootInput.ArrayConst[0]
            else
                rootInput;
        if traverse.Type == "attr" then
            local rootObj = barbe.asVal(root);
            if !std.isObject(rootObj) then
                error "<showuser>cannot find attribute '" + traverse.Name + "' on non-object (" + std.get(root, "Type", "?") + ") '" + errorPrefix +"'</showuser>"
            else if !std.objectHas(rootObj, traverse.Name) then
                error "<showuser>cannot find attribute '" + traverse.Name + "' on object '" + errorPrefix +"'</showuser>"
            else
                rootObj[traverse.Name]
        else if traverse.Type == "index" then
            if std.isString(traverse.Index) then
                barbe.lookupTraverse(root, {Type: "attr", Name: traverse.Index}, errorPrefix)
            else
                local rootArr = barbe.asVal(root);
                if !std.isArray(rootArr) then
                    error "<showuser>cannot find index '" + traverse.Index + "' on non-array '" + errorPrefix +"'</showuser>"
                else if std.length(rootArr) <= traverse.Index || traverse.Index < 0 then
                    error "<showuser>index '" + traverse.Index + "' is out of bounds on '" + errorPrefix +"'</showuser>"
                else
                    rootArr[traverse.Index]
        else
            error errorPrefix + ": invalid traversal type '" + traverse.Type + "'"
    ,

    // lookup the value of a full traversal on an object or array
    // root is a syntax token of type object_const or array_const
    // traverseArr is and array of Traverse objects, typically from a scope_traversal token
    lookupTraversal(root, traverseArr, errorPrefix)::
        if std.length(traverseArr) == 0 then
            root
        else if std.length(traverseArr) == 1 then
            barbe.lookupTraverse(root, traverseArr[0], errorPrefix)
        else
            local debugStr = barbe.asStr({Type: "scope_traversal", Traversal: [traverseArr[0]]});
            barbe.lookupTraversal(
                barbe.lookupTraverse(root, traverseArr[0], errorPrefix),
                traverseArr[1:],
                errorPrefix + (if std.startsWith(debugStr, "[") then "" else ".") + debugStr
            )
    ,

    asStr(token)::
        if std.isString(token) then
            token
        else if token.Type == "scope_traversal" then
            std.join("", [
                local traverse = token.Traversal[i];
                if traverse.Type == "attr" then
                    traverse.Name +
                    (if i == std.length(token.Traversal)-1 || token.Traversal[i+1].Type != "attr" then "" else ".")
                else
                    "[" +
                    (if std.isString(traverse.Index) then "\"" else "") +
                    traverse.Index +
                    (if std.isString(traverse.Index) then "\"" else "") +
                    "]" +
                    (if i == std.length(token.Traversal)-1 || token.Traversal[i+1].Type != "attr" then "" else ".")
                for i in std.range(0, std.length(token.Traversal)-1)
            ])
        else if token.Type == "literal_value" then
            token.Value+""
        else if token.Type == "template" then
            std.join("", [barbe.asStr(part) for part in token.Parts])
        ,

    mergeTokens(values)::
        if std.length(values) == 0 then
            barbe.asSyntax({})
        else if std.length(values) == 1 then
            values[0]
        else
            if values[0] == null then
               error "<showuser>tried to merge null value</showuser>"
            else if values[0].Type == "literal_value" then
                values[std.length(values)-1]
            else if values[0].Type == "array_const" then
                {
                    Type: "array_const",
                    ArrayConst: barbe.flatten([
                        std.get(value, "ArrayConst", []) for value in values
                    ])
                }
            else if values[0].Type == "object_const" then
                local allObjConst = barbe.flatten([
                    std.get(value, "ObjectConst", []) for value in values
                ]);
                local v = {
                    [allObjConst[i].Key]: barbe.mergeTokens([v.Value for v in allObjConst if v.Key == allObjConst[i].Key])
                    for i in std.range(0, std.length(allObjConst)-1)
                    if !std.member(
                        [item.Key for item in allObjConst[i+1:std.length(allObjConst)]],
                        allObjConst[i].Key
                    )
                };
                {
                    Type: "object_const",
                    //merge all the keys that are the same
                    ObjectConst: [
                        { Key: key, Value: v[key] }
                        for key in std.objectFields(v)
                    ]
                }
            else
                values[std.length(values)-1]
        ,

    asVal(token)::
        if token.Type == "template" then
            std.join("", [barbe.asStr(part) for part in token.Parts])
        else if token.Type == "literal_value" then
            std.get(token, "Value", null)
        else if token.Type == "array_const" then
            std.get(token, "ArrayConst", [])
        else if token.Type == "object_const" then
            local keys = [pair.Key for pair in std.get(token, "ObjectConst", [])];
            local uniqKeys = std.set(keys);
            local allValues(key) = [pair.Value for pair in std.get(token, "ObjectConst", []) if pair.Key == key];
            {
                [key]: barbe.mergeTokens(allValues(key))
                for key in uniqKeys
            }
        ,

    asValArrayConst(token):: [barbe.asVal(item) for item in barbe.asVal(token)],

    asSyntax(token)::
        if std.isObject(token) && std.objectHas(token, "Type") then
            token
        else if std.isString(token) || std.isNumber(token) || std.isBoolean(token) then
            {
                Type: "literal_value",
                Value: token
            }
        else if std.isArray(token) then
            {
                Type: "array_const",
                ArrayConst: [barbe.asSyntax(child) for child in token if child != null]
            }
        else if std.isObject(token) then
            {
                Type: "object_const",
                ObjectConst: [
                    {
                        Key: key,
                        Value: barbe.asSyntax(token[key])
                    }
                    for key in std.objectFields(token)
                ]
            }
        else
            token
            //error "unknown asSyntax token: " + token
        ,

    asTraversal(str):: {
        Type: "scope_traversal",
        Traversal: [
            // TODO this will output correct string for indexing ("abc[0]") but
            // is using the wrong syntax token (Type: "attr" instead of Type: "index")
           {
                Type: "attr",
                Name: part
           }
           for part in std.split(str, ".")
        ]
    },

    appendToTraversal(source, toAdd):: {
            Type: source.Type,
            Traversal: source.Traversal + [
                {
                    Type: "attr",
                    Name: part
                }
                for part in std.split(toAdd, ".")
            ]
        },

    asFuncCall(funcName, args):: {
            Type: "function_call",
            FunctionName: funcName,
            FunctionArgs: [barbe.asSyntax(item) for item in args]
        },

    asTemplate(arr):: {
            Type: "template",
            Parts: [barbe.asSyntax(item) for item in arr]
        },

    // turns an array of various syntax types into a templated string
    asTemplateStr(arr):: {
            Type: "template",
            Parts: [
              if item.Type == "scope_traversal" || item.Type == "relative_traversal" || item.Type == "literal_value" || item.Type == "template" then
                  item
              else
                  barbe.asStr(item)
              for item in arr
            ]
        },

    //string concatenation for syntax tokens
    concatStrArr(token):: {
            Type: "template",
            Parts: barbe.flatten(barbe.asTemplateStr(std.get(token, "ArrayConst", [])).Parts)
        },

    appendToTemplate(source, toAdd):: {
            Type: "template",
            Parts: barbe.flatten([
                if source.Type == "template" then
                    source.Parts
                else if source.Type == "literal_value" then
                    source
                else
                    source
            ] + [barbe.asSyntax(item) for item in toAdd])
        },

    asBlock(arr):: {
            Type: "array_const",
            Meta: { IsBlock: true },
            ArrayConst: [
                {
                    Type: "object_const",
                    Meta: { IsBlock: true },
                    ObjectConst: [
                        {
                            Key: key,
                            Value: barbe.asSyntax(obj[key])
                        }
                        for key in std.objectFields(obj)
                    ]
                }
                for obj in arr
            ]
        },

    removeLabels(obj):: {
            [key]: obj[key]
            for key in std.objectFields(obj)
            if key != "labels"
        },

    iterateAllBlocks(container, func):: [
            func(block)
            for type in std.objectFields(container)
            for blockName in std.objectFields(container[type])
            for block in container[type][blockName]
        ],

    iterateBlocks(container, ofType, func):: [
            func(block)
            for blockName in std.objectFields(std.get(container, ofType, {}))
            for block in container[ofType][blockName]
        ],

    compileDefaults(container, name)::
        local blocks = barbe.flatten([
            if std.objectHas(container, "global_default") then
                [
                    block.Value
                    for group in std.objectValues(container.global_default)
                    for block in group
                ],
            if std.objectHas(container, "default") && std.objectHas(container.default, name) then
                [block.Value for block in container.default[name]]
        ]);
        barbe.asVal({
            Type: "object_const",
            ObjectConst: barbe.flatten([std.get(block, "ObjectConst", []) for block in blocks if block != null])
        }),

    makeBlockDefault(container, globalDefaults, block)::
        if std.objectHas(block, "copy_from") then
            barbe.compileDefaults(container, barbe.asStr(block.copy_from))
        else
            globalDefaults
        ,

    cloudResourceRaw(dir, id, kind, type, name, value):: {
        Type: "cr_" + (
            if kind != null then
                (
                    "[" + kind + (
                        if id != null then "(" + id + ")" else ""
                    ) +
                    "]" +
                    (if type != null then "_" else "")
                )
            else ""
        ) + (if type != null then type else ""),
        Name: name,
        Value: value,
    } + if dir != null then { Meta: { sub_dir: dir } } else {},

};
barbe
