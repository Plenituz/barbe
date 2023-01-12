import { Console } from "as-wasi/assembly";
import { JSON } from "json-as/assembly";

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
    Console.log(msg);
    const rawResp = Console.readLine();
    return JSON.parse<RpcResponse<Result>>(rawResp!);
}

let resp = rpcCall<i32[], i32>({
    method: "add",
    params: [1, 5]
});

Console.log('parsed: ' + resp.result.toString());

