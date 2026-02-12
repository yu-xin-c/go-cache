namespace go mygocache

struct Request {
    1: string group
    2: string key
}

struct Response {
    1: binary value
}

struct SetRequest {
    1: string group
    2: string key
    3: binary value
    4: i64 ttl
}

struct SetResponse {
    1: bool success
}

struct DeleteRequest {
    1: string group
    2: string key
}

struct DeleteResponse {
    1: bool success
}

struct ClearRequest {
    1: string group
}

struct ClearResponse {
    1: bool success
}

struct StatsRequest {
    1: string group
}

struct StatsResponse {
    1: i64 itemCount
    2: i64 hitCount
    3: i64 missCount
    4: i64 totalCount
}

struct GetMultiRequest {
    1: string group
    2: list<string> keys
}

struct GetMultiResponse {
    1: map<string, binary> values
}

struct SetMultiRequest {
    1: string group
    2: map<string, binary> values
    3: i64 ttl
}

struct SetMultiResponse {
    1: bool success
}

service GroupCache {
    Response Get(1: Request req)
    SetResponse Set(1: SetRequest req)
    DeleteResponse Delete(1: DeleteRequest req)
    ClearResponse Clear(1: ClearRequest req)
    StatsResponse Stats(1: StatsRequest req)
    GetMultiResponse GetMulti(1: GetMultiRequest req)
    SetMultiResponse SetMulti(1: SetMultiRequest req)
}
