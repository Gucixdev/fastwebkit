# chat (TypeScript)

The client side of a chat: a message store as a tree (editing or forking a message starts a sibling branch, the
active leaf decides which line of conversation you see), a bare bubble renderer with an optional per-message actions
hook, and end-to-end encryption with WebCrypto (ECDH key pair, shared key, AES-GCM). The server side of a chat room
is in `templates/chat`; it does not use this module yet (it stores plain text).

Depends on: nothing in this repo. Browser APIs only (DOM, `crypto.subtle`, clipboard).

## Interface

    chat.ts         ChatStore (add, get, childrenOf, getThread, fork), ChatView, copyText, copyToClipboard, actionButton
    chat_crypto.ts  generateKeyPair, exportPublicKey, importPublicKey, deriveSharedKey, encryptMessage, decryptMessage

Per-message actions (copy, fork, regenerate, delete, react) are an app decision: `ChatView` takes a `renderActions` hook.

## Test

    cd fastwebapp && npm test        # tsc --noEmit, then vitest for every module
