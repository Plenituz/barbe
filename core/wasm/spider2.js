

function rpcCall(req) {
    const msg = JSON.stringify(req)
    console.log('sending: ' + msg)
    console.log(msg)
    const rawResp = readline()
    return JSON.parse(rawResp)
}

function sleep(ms) {
    rpcCall({
        method: "sleep",
        params: [ms]
    });
}

async function main() {
    console.log('spider1.js loaded')


    console.log('done waiting')

    const resp = rpcCall({
        method: "exportDatabags",
        params: [{
            databag1: {
                key1: {
                    Type: "oy",
                    Value: {
                        a: 1,
                    }
                },
            }
        }]
    });
    console.log(JSON.stringify(resp))
}

main()