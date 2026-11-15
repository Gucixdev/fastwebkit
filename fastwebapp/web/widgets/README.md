# widgets (TypeScript)

Small independent UI pieces. They do not import each other: copy one file and its test.

Depends on: nothing. Browser DOM only (`theme.ts` also uses `localStorage`, guarded).

## Interface

    notifications.ts   NotificationCenter (info, success, error toasts)
    progressbar.ts     ProgressBar, clampPercent
    searchbar.ts       SearchBar, matches, filterByQuery, debounce
    spring.ts          Spring (damped spring animation)
    theme.ts           ThemeController, nextTheme, readStoredTheme, storeTheme
    upload.ts          UploadZone (drag and drop, posts multipart/form-data), validateFile, formatBytes

`UploadZone` posts to the `url` you give it; point it at the `downloads` Go module's `POST /downloads`.

## Test

    cd fastwebapp && npm test
