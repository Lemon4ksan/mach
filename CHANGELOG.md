# Changelog

## [0.1.0](https://github.com/Lemon4ksan/mach/compare/v0.0.1...v0.1.0) (2026-10-04)


### Features

* adapt QPACKCodec to idiomatic callback-based QPACK engine API ([c301fac](https://github.com/Lemon4ksan/mach/commit/c301fac24d7a1665b7d666d5fc8a161e6cd722f2))
* **ci:** add github workflows, fuzzing suite and fix quic config export ([40c8fe2](https://github.com/Lemon4ksan/mach/commit/40c8fe2517de11675db71a78f72f8b82434ac614))
* implement RaptorQ extension over QUIC datagrams ([b3b449e](https://github.com/Lemon4ksan/mach/commit/b3b449e7eba587837c1b680842c0c688c9d88512))
* integrate Chromium QPACK dynamic table engine into HTTP/3 client and server ([cc4b7c6](https://github.com/Lemon4ksan/mach/commit/cc4b7c6f8f158a62bd33137a26c5e4cdb4d57fd4))
* integrate L7 protocols, frame mechanics, and csrc vector kernels ([27367ef](https://github.com/Lemon4ksan/mach/commit/27367ef4a886fbdf58c8d45ba35559329afbf8fe))
* migrate hpack and qpack from foundation ([8385869](https://github.com/Lemon4ksan/mach/commit/8385869aab462be46b8b5072ae37b71f6f863162))
* **quic:** add PATH_ABANDON frame and PathScheduler interface for ([644686f](https://github.com/Lemon4ksan/mach/commit/644686f4b62dfbebe77b766fe6b3fea00cfec899))
* **server/h1:** implement zero-allocation pipelining and simd fast-paths ([5cd08f1](https://github.com/Lemon4ksan/mach/commit/5cd08f13c00954590afefafd35e4089179bea2c5))
* **simd:** accelerate quicvarint and h1 parsing ([b287d66](https://github.com/Lemon4ksan/mach/commit/b287d66929885b62fe08aee39d4a1e2c47423b42))


### Bug Fixes

* apply Round 3 fixes for HTTP/1.1, HTTP/2, and HTTP/3 ([33773d3](https://github.com/Lemon4ksan/mach/commit/33773d3b1e048138a0e74f95af87b7645599b3ef))
* **ci:** resolve missing 13 errcheck linters and setup root package ([be96d3b](https://github.com/Lemon4ksan/mach/commit/be96d3b6aa6e187b16f13c6c29a773094d6847f0))
* **client/h1:** fallback to default net.Dial when dialer is unspecified ([9f5019b](https://github.com/Lemon4ksan/mach/commit/9f5019bc6c338b328589b3746bdda45d520c8e61))
* fuzzing and h2 table max cap ([bb7ae07](https://github.com/Lemon4ksan/mach/commit/bb7ae07718719d05c3e424a04599b410480b1803))
* **h2:** enforce MaxCapacity on HPACK dynamic table size updates ([8aea428](https://github.com/Lemon4ksan/mach/commit/8aea4287b8d38b7f70077129a9753f14e779ffd7))
* **h3:** enforce setting bounds per RFC 9297 and RFC 9220 ([a9e77b8](https://github.com/Lemon4ksan/mach/commit/a9e77b8c0982ecae62759a4a762b45ea99ef508c))
* HTTP protocol inaccuracies vs Chromium ([11aeffd](https://github.com/Lemon4ksan/mach/commit/11aeffdccf27c54ec835fe130714be0882f482fc))
* HTTP/2 frames test case to comply with strict stream ID and frame size requirements ([5225286](https://github.com/Lemon4ksan/mach/commit/5225286aa310722534ed2289de6bdb64b77608a0))
* HTTP/2, HTTP/3, and HTTP/1.1 protocol deviations ([29ebd27](https://github.com/Lemon4ksan/mach/commit/29ebd2705b409e5e26bd5265c5f08ab2dda0fbd5))
* **http:** prevent CRLF injection in header serialization ([bd13fa3](https://github.com/Lemon4ksan/mach/commit/bd13fa3e3428ba057630d0a604f33ff7f5113019))
* **http:** prevent request smuggling and chunk size DoS ([3d78424](https://github.com/Lemon4ksan/mach/commit/3d7842450e0092e830cf2e187509ffef5691e9ee))
* **lint,race:** resolve 70+ golangci-lint violations and fix conn state data races ([d9a95ce](https://github.com/Lemon4ksan/mach/commit/d9a95ce7a3b459ea94d313713ede0f4329589781))
* **quic:** drop flaky concurrent close test ([d3bb839](https://github.com/Lemon4ksan/mach/commit/d3bb839e363f2b30d080f8a5e464a9400e96de8b))
* resolve failing smoke benchmarks and sync README structure ([3e5b5d7](https://github.com/Lemon4ksan/mach/commit/3e5b5d729573f79f5d1f532450eb1fc6aaf39486))
* **test:** wrap first NewClientConn in recover to fix h3 panic on nil interface ([d2a2281](https://github.com/Lemon4ksan/mach/commit/d2a2281b0abb32983095cfcc4784ac86049424fb))
* update ast and bin imports to new foundation namespaces ([e7e9fc6](https://github.com/Lemon4ksan/mach/commit/e7e9fc69c11eced822ee270cea252e6b95849a77))


### Performance Improvements

* achieve 0 allocations in QPACK decoder with request arena ([40b235b](https://github.com/Lemon4ksan/mach/commit/40b235b76bd0a9eea92c2634a7001d33be47f94a))
