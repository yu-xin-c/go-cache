namespace go geecache

struct Request {
    1: string group
    2: string key
}

struct Response {
    1: binary value
}

service GroupCache {
    Response Get(1: Request req)
}
