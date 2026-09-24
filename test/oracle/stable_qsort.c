/*
 * Stable qsort/qsort_r for the Rails oracle.
 *
 * Ruby delegates Array#sort and Enumerable#sort_by to the C library's
 * qsort_r. Production runs on Debian bookworm (glibc 2.36), whose qsort_r is
 * a merge sort and therefore stable; glibc >= 2.37 switched to introsort,
 * which is not. Preloading this file gives a local Ruby the same (stable)
 * ordering of equal keys that production shows.
 *
 *   gcc -shared -fPIC -O2 -o stable_qsort.so stable_qsort.c
 */
#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>

typedef int (*cmp_r_t)(const void *, const void *, void *);

static void merge_sort(char *base, char *tmp, size_t n, size_t size, cmp_r_t cmp, void *arg)
{
    if (n < 2) return;
    size_t n1 = n / 2, n2 = n - n1;
    char *b1 = base, *b2 = base + n1 * size;
    merge_sort(b1, tmp, n1, size, cmp, arg);
    merge_sort(b2, tmp, n2, size, cmp, arg);
    char *out = tmp;
    while (n1 > 0 && n2 > 0) {
        if (cmp(b1, b2, arg) <= 0) { memcpy(out, b1, size); b1 += size; n1--; }
        else { memcpy(out, b2, size); b2 += size; n2--; }
        out += size;
    }
    if (n1 > 0) { memcpy(out, b1, n1 * size); out += n1 * size; }
    memcpy(base, tmp, (size_t)(out - tmp));
}

void qsort_r(void *base, size_t nmemb, size_t size, cmp_r_t cmp, void *arg)
{
    if (nmemb < 2 || size == 0) return;
    char *tmp = malloc(nmemb * size);
    if (!tmp) abort();
    merge_sort(base, tmp, nmemb, size, cmp, arg);
    free(tmp);
}

struct plain { int (*cmp)(const void *, const void *); };
static int call_plain(const void *a, const void *b, void *arg)
{
    return ((struct plain *)arg)->cmp(a, b);
}

void qsort(void *base, size_t nmemb, size_t size, int (*cmp)(const void *, const void *))
{
    struct plain p = { cmp };
    qsort_r(base, nmemb, size, call_plain, &p);
}
