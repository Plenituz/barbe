package barbe

import (
	"strings"
	"list"
)

#AsStr: {
	#In: _

	if #In.Type == "scope_traversal" {
		out: strings.Join([
				for traverse in #In.Traversal {
					traverse.Name
				}
			], ".")
	}

	if #In.Type == "literal_value" {
		out: #In.Value + ""
	}

	if #In.Type == "template" {
		out: strings.Join([
				for part in #In.Parts {
					(#AsStr & {#In: part}).out
				}
			], "")
	}
}

#TraversalAsStr: {
	#In: {Type: "scope_traversal" | "relative_traversal", Traversal: [..._]}

	out: strings.Join([
			for traverse in #In.Traversal {
				[
					if traverse.Type == "attr" {
						traverse.Name
					},
					if traverse.Type == "index" {
						"[\(traverse.Index)]"
					}
				][0]
			}
		], ".")
}


// WARNING: this is a huge hit on performance even for simple objects, concider unrolling
#AsSyntax: {
	#In: _

	//this early out prevent cue from going into crazy recursion delay
	if #In.Type != _|_ {
		out: #In
	}

	if #In.Type == _|_ {
		out: [
				if #In.Type == _|_ && (#In & string) != _|_ || (#In & number) != _|_ || (#In & bool) != _|_ {
					{
						Type: "literal_value"
						Value: #In
					}
				},

				if (#In & [..._]) != _|_ {
					{
						Type: "array_const"
						ArrayConst: [
							for value in #In if value != _|_ {
								//we have to use null coaslescing here to make cue allow this
								//otherwise it thinks it's a structural cycle
								*(#AsSyntax & {#In: value}).out | null
							}
						]
					}
				},

				if (#In & {}) != _|_ {
					{
						Type: "object_const"
						ObjectConst: [
							for key, value in #In {
								{
									Key: key
									Value: *(#AsSyntax & {#In: value}).out | null
								}
							}
						]
					}
				},

				//default
				#In
		][0]
	}


}

#AsVal: {
	#In: _

	if #In.Type == "template" {
		out: strings.Join([
				for part in #In.Parts {
					(#AsStr & {#In: part}).out
				}
			], "")
	}

	if #In.Type == "literal_value" {
		out: #In.Value
	}

	if #In.Type == "array_const" {
		out: #In.ArrayConst
	}

	if #In.Type == "object_const" {
		let keys = { for i in [for pair in #In.ObjectConst {pair.Key}] { "\(i)": i } }
		let uniqKeys = [ for i, j in keys { j } ]

		out: {
				for key in uniqKeys {
					let allValues = [
						for pair in #In.ObjectConst if pair.Key == key {
							pair.Value
						}
					]
					if len(allValues) == 1 {
						"\(key)": allValues[0]
					}
					if len(allValues) > 1 {
						[
							if allValues[0].Type == "literal_value" {
								"\(key)": allValues
							},
							if allValues[0].Type == "array_const" {
								"\(key)": {
									Type: "array_const"
									ArrayConst: list.FlattenN([
										for value in allValues {value.ArrayConst}
									], 3)
								}
							},
							if allValues[0].Type == "object_const" {
								"\(key)": {
									Type: "object_const"
									ObjectConst: list.FlattenN([
										for value in allValues {value.ObjectConst}
									], 3)
								}
							},
							//default, return the last value
							{"\(key)": allValues[len(allValues)-1]}
						][0]
					}
				}
			}
	}
	if #In.Type != "template" && #In.Type != "literal_value" && #In.Type != "array_const" && #In.Type != "object_const" {
		out: #In
	}
}

#RemoveLabels: {
	#In: [string]: _

	{
		for key, value in #In {
			if key != "labels" {
				"\(key)": value
			}
		}
	}
}

#AsValArrayConst: {
	#In: _
	let t = #In
	out: [
		for item in (#AsVal & {#In: t}).out {
			(#AsVal & {#In: item}).out
		}
	]
}

#AsTraversal: {
	#In: string

	{
		Type: "scope_traversal"
		Traversal: [
			for part in strings.Split(#In, ".") {
				// TODO add support for indexing
				{
					Type: "attr",
					Name: part
				}
			}
		]
	}
}

#AppendToTraversal: {
	#Source: _
	#ToAdd: string

	{
		Type: "relative_traversal",
		Source: #Source,
		Traversal: [
			for part in strings.Split(#ToAdd, ".") {
				// TODO add support for indexing
				{
					Type: "attr",
					Name: part
				}
			}
		]
	}
}

#AsFuncCall: {
	#FuncName: string
	#Args: [..._]

	out: {
		Type: "function_call"
		FunctionName: #FuncName,
		FunctionArgs: [
			for item in #Args {
				*(#AsSyntax & {#In: item}).out | null
			}
		]
	}
}

#AsTemplate: {
	#In: [..._]

	{
		Type: "template"
		Parts: [
			for item in #In {
				*(#AsSyntax & {#In: item}).out | null
			}
		]
	}
}

#AsBlock: {
	#In: [..._]

	out: {
		Type: "array_const"
		Meta: IsBlock: true
		ArrayConst: [
				for item in #In {
					{
						Type: "object_const"
						Meta: IsBlock: true
						ObjectConst: [
							for key, value in item {
									{
										Key: key,
										if value.Type != _|_ {
											Value: value
										}
										if value.Type == _|_ {
											Value: *(#AsSyntax & {#In: value}).out | null
										}
									}
							}
						]
					}
				}
			]
	}
}

#FieldList: {
	#Struct: [string]: _

	out: [
		for field, value in #Struct {
			field
		}
	]
}

#FieldListArr: {
	#StructList: [...{[string]: _}]

	out: list.FlattenN([
			for struct in #StructList {
				(#FieldList & {#Struct: struct}).out
			}
	], 2)
}

#MergeAllStructs: {
	#In: [...{[string]: _}]

	{
		for currentIndex, currentStruct in #In {
			for currentField, currentValue in currentStruct {
				// add anything not in all other structs that come later in the list
				if !list.Contains((#FieldListArr & {#StructList: #In[currentIndex+1:len(#In)]}).out, currentField) {
					"\(currentField)": currentValue
				}
			}
		}
	}
}

#CompileDefaults: {
	#Container: _
	#Name: string

	 out: (#AsVal & {
		#In: {
			Type: "object_const"
			ObjectConst: list.FlattenN([
					for block in list.FlattenN([
							if #Container["global_default"] != _|_ {
								[for key, value in #Container["global_default"] {
									value
								}]
							},
							if #Container["default"] != _|_ && #Container["default"][#Name] != _|_ {
								#Container["default"][#Name]
							}
						], 4) {
						if block != _|_ {
							block.ObjectConst
						}
					}
				], 2)
		}
	}).out
}

// turns an array of various syntax types into a templated string
#AsTemplateStr: {
	#In: [..._]

	out: {
		Type: "template"
		Parts: [
			for item in #In {
				[
					if item.Type == "scope_traversal" || item.Type == "relative_traversal" || item.Type == "literal_value" || item.Type == "template" {
						item
					},
					(#AsStr & {#In: item}).out
				][0]
			}
		]
	}
}

#AppendToTemplate: {
	#Source: _
	#ToAdd: [..._]

	out: {
		Type: "template"
		Parts: list.FlattenN([
			if #Source.Type == "template" {
				[
					for item in #Source.Parts {
						item
					}
				]
			},
			if #Source.Type == "literal_value" {
				#Source
			},
			[
				for item in #ToAdd {
					(#AsSyntax & {#In: item}).out
				}
			]
		], 2)
	}
}

#ConcatStrArr: {
	#In: {Type: "array_const", [string]: _}

	let t = #In
	let rawTemplate = (#AsTemplateStr & {#In: t.ArrayConst}).out
	let parts = list.FlattenN([
		for i, item in rawTemplate.Parts {
			item
		}
	], 2)
	out: {
		Type: "template"
		Parts: *parts | []
	}
}

#UniqList: {
	#In: [..._]

	out: [ for i, x in #In if !list.Contains(list.Drop(#In, i+1), x) {x}]
}

#ReverseList: {
	#In: [..._]

	out: [
		for i in list.Range(len(#In)-1, -1, -1) {
			#In[i]
		}
	]
}

#ReverseString: {
	#In: string

	let t = #In
	out: strings.Join((#ReverseList & {#In: strings.Split(t, "")}).out, "")
}