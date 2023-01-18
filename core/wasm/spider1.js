

function rpcCall(req) {
    const msg = JSON.stringify(req)
    console.log(msg)
    const rawResp = readline()
    return JSON.parse(rawResp)
}

function sleep(ms) {
    rpcCall({
        method: "sleep",
        params: [ms]
    })
}

function exportDatabags(bags) {
    const resp = rpcCall({
        method: "exportDatabags",
        params: [{
            databags: bags
        }]
    });
    if(resp.error) {
        throw new Error(resp.error)
    }
}

async function main() {
    exportDatabags([
        {
            Type: "spider1",
            Name: "spider1",
            Value: {
                a: 'b'
            }
        }
    ])
}

main()