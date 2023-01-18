import { Console } from "as-wasi/assembly";
import { JSON } from "../../../../as-json/assembly";

@JSON
class RpcRequest<Params> {
    method!: string;
    params!: Params;
}

@JSON
class RpcResponse<Result> {
    result!: Result;
    error!: string | null;
}

function rpcCall<Params, Result>(req: RpcRequest<Params>): RpcResponse<Result> {
    const msg = JSON.stringify(req);
    Console.log('sending: ' + msg);
    Console.log(msg);
    const rawResp = Console.readLine();
    return JSON.parse<RpcResponse<Result>>(rawResp!);
}

// let req = new Map<string, Map<string, string>>();
// let bag1 = new Map<string, string>();
// bag1.set("a", "1");
// req.set("databag1", bag1);
function canSerde<T>(data: T): void {
    const serialized = JSON.stringify<T>(data);
    const deserialized = JSON.stringify<T>(JSON.parse<T>(serialized));
    Console.log(deserialized);
}


let map1 = new Map<string, string>();
map1.set("a", "1");
map1.set("b", "2");
canSerde<Map<string, string>>(map1);

let map2 = new Map<string, string>();
map2.set("a", "1");
canSerde<Map<string, string>>(map2);

let root = new Map<string, Map<string, Map<string, i32>>>();
let map3 = new Map<string, Map<string, i32>>();
let map4 = new Map<string, i32>();
map4.set("a", 1);
map4.set("b", 2);
map3.set("c", map4);
root.set("d", map3);
canSerde<Map<string, Map<string, Map<string, i32>>>>(root);

// let resp = rpcCall<Map<string, Map<string, string>>[], i32>({
//     method: "exportDatabags",
//     params: [req]
// });
//
// Console.log('parsed: ' + resp.result.toString());

