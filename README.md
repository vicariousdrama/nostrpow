# nostrpow

A simple tool for taking a prepared event in JSON format, and attempting to find a suitable proof of work nonce to generate a desired note id.

## Building

With go installed, simply run

```sh
go build
```

This will produce the `nostrpow` binary that can be run

## Running

Arguments are by ordinal position, and are not named.  If wanting to specify a later positioned argument, all prior ones must be given

| Position | Required? | Data Type | Description |
| --- | --- | --- | --- |
| 1 | required | String | Input filename.  The path to a json file that contains requisite fields filled in: pubkey, created_at, kind, tags, content |
| 2 | optional | Number | Target Proof of Work Level. If not specified, and not set as a tag within the input file, the default will be 24. This value indicates the number of leading zeros that are desired for the note id when represented in binary.  Maximum allowed value is 64. |
| 3 | optional | Number | Starting Nonce value. To support batching, A nonce value may be indicated to start from. |
| 4 | optional | Number | Number of routines to run simultaenously. By default this will be set to two less than the number of CPUs on the system. |
| 5 | optional | Number | Batch Size. If set, overrides the default batch size of 100 billion. This is the total number of nonce checks that will be tried for the batch. It will be divided by teh number of simultaneously run routines |

Example
```sh
./nostrpow nostrevent.json 42 1100000000000 6 50000000000
```

## About

Based in part on 

- [SignAndSend](https://github.com/vicariousdrama/SignAndSend) python script written by vicariousdrama - MIT License
- [Vainstr](https://github.com/mleku/vainstr) go code written by Mleku - Creative Commons License, and uses interrupt and qu projects from mleku.online/git

