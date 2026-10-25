// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cgo

#include <jni.h>
#include <stdlib.h>

extern int WudictGoBridgeProtocol(void);
extern char *WudictGoBridgeConfigSummary(char *path, char *flagsJSON);
extern void WudictGoBridgeInstallBusyObserver(void);
extern int WudictGoBridgeRuntimeStart(char *path, char *flagsJSON, char *version);
extern int WudictGoBridgeRuntimeStop(void);
extern char *WudictGoBridgeConfigSnapshot(void);
extern char *WudictGoBridgePrefsSnapshot(void);
extern char *WudictGoBridgeBrowsePage(char *query);
extern char *WudictGoBridgeBrowseFindPage(char *query);
extern char *WudictGoBridgeBrowseDiagnostic(char *payload);
extern char *WudictGoBridgeAbout(char *origin, char *dictionary);
extern char *WudictGoBridgeOrphans(char *action, char *body);
extern char *WudictGoBridgeRemoveLibrary(char *query);
extern char *WudictGoBridgeLooks(char *argument);
extern char *WudictGoBridgeGroups(char *action, char *argument);
extern char *WudictGoBridgeLemmas(char *action, char *code);
extern char *WudictGoBridgeSetup(char *action, char *argument);
extern char *WudictGoBridgeHeadwordExport(char *action, char *argument);
extern char *WudictGoBridgeIntake(char *action, char *argument);
extern int WudictGoBridgePower(char *state);
extern char *WudictGoBridgeReindex(char *action);
extern char *WudictGoBridgeIndexWork(char *action, char *id, char *body);
extern char *WudictGoBridgePresetsSnapshot(void);
extern char *WudictGoBridgeSavePreset(char *body);
extern char *WudictGoBridgeStyleSnapshot(void);
extern char *WudictGoBridgeSaveStyle(char *argument);
extern char *WudictGoBridgeUserFilesSnapshot(void);
extern char *WudictGoBridgeUserFileOperation(char *method, char *argument);
extern char *WudictGoBridgeUserStorage(char *method, char *argument);
extern char *WudictGoBridgeSavePrefs(char *body);
extern char *WudictGoBridgeSaveLanguage(char *body);
extern char *WudictGoBridgeSystemLogSnapshot(void);
extern int WudictGoBridgeSystemLogOptions(int enabled, int baseEvents, int webEvents);
extern char *WudictGoBridgeWebResource(char *rawURL, char *headers);
extern char *WudictGoBridgeMediaOpen(char *rawURL, char *headers);
extern int WudictGoBridgeMediaRead(long long handle, char *dst, int size);
extern void WudictGoBridgeMediaClose(long long handle);
extern int WudictGoBridgeDictStreamStart(void);
extern int WudictGoBridgeRescanStreamStart(void);
extern int WudictGoBridgeRescanIndexesStart(char *origin, char *request);
extern int WudictGoBridgeIngestStart(char *origin, char *query);
extern int WudictGoBridgeSearchStreamStart(char *origin, char *request);
extern int WudictGoBridgeBrowseStreamStart(char *method, char *query);
extern char *WudictGoBridgeDictStreamPoll(int id);
extern void WudictGoBridgeDictStreamCancel(int id);

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeDictStreamStart(JNIEnv *env, jclass owner) {
    (void)env; (void)owner;
    return (jint)WudictGoBridgeDictStreamStart();
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeRescanStreamStart(JNIEnv *env, jclass owner) {
    (void)env; (void)owner;
    return (jint)WudictGoBridgeRescanStreamStart();
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeRescanIndexesStart(JNIEnv *env, jclass owner,
    jstring origin, jstring request) {
    (void)owner;
    if (origin == NULL || request == NULL) return -1;
    const char *base = (*env)->GetStringUTFChars(env, origin, NULL);
    if (base == NULL) return -1;
    const char *body = (*env)->GetStringUTFChars(env, request, NULL);
    if (body == NULL) {
        (*env)->ReleaseStringUTFChars(env, origin, base);
        return -1;
    }
    jint result = (jint)WudictGoBridgeRescanIndexesStart((char *)base, (char *)body);
    (*env)->ReleaseStringUTFChars(env, request, body);
    (*env)->ReleaseStringUTFChars(env, origin, base);
    return result;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeIngestStart(JNIEnv *env, jclass owner,
    jstring origin, jstring query) {
    (void)owner;
    if (origin == NULL || query == NULL) return -1;
    const char *base = (*env)->GetStringUTFChars(env, origin, NULL);
    if (base == NULL) return -1;
    const char *values = (*env)->GetStringUTFChars(env, query, NULL);
    if (values == NULL) {
        (*env)->ReleaseStringUTFChars(env, origin, base);
        return -1;
    }
    jint result = (jint)WudictGoBridgeIngestStart((char *)base, (char *)values);
    (*env)->ReleaseStringUTFChars(env, query, values);
    (*env)->ReleaseStringUTFChars(env, origin, base);
    return result;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSearchStreamStart(JNIEnv *env, jclass owner,
    jstring origin, jstring request) {
    (void)owner;
    if (origin == NULL || request == NULL) return -1;
    const char *base = (*env)->GetStringUTFChars(env, origin, NULL);
    if (base == NULL) return -1;
    const char *query = (*env)->GetStringUTFChars(env, request, NULL);
    if (query == NULL) {
        (*env)->ReleaseStringUTFChars(env, origin, base);
        return -1;
    }
    jint result = (jint)WudictGoBridgeSearchStreamStart((char *)base, (char *)query);
    (*env)->ReleaseStringUTFChars(env, request, query);
    (*env)->ReleaseStringUTFChars(env, origin, base);
    return result;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeBrowseStreamStart(JNIEnv *env, jclass owner,
    jstring method, jstring request) {
    (void)owner;
    if (method == NULL || request == NULL) return -1;
    const char *name = (*env)->GetStringUTFChars(env, method, NULL);
    if (name == NULL) return -1;
    const char *query = (*env)->GetStringUTFChars(env, request, NULL);
    if (query == NULL) {
        (*env)->ReleaseStringUTFChars(env, method, name);
        return -1;
    }
    jint result = (jint)WudictGoBridgeBrowseStreamStart((char *)name, (char *)query);
    (*env)->ReleaseStringUTFChars(env, request, query);
    (*env)->ReleaseStringUTFChars(env, method, name);
    return result;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeDictStreamPoll(JNIEnv *env, jclass owner, jint id) {
    (void)owner;
    char *result = WudictGoBridgeDictStreamPoll((int)id);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT void JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeDictStreamCancel(JNIEnv *env, jclass owner, jint id) {
    (void)env; (void)owner;
    WudictGoBridgeDictStreamCancel((int)id);
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeBrowsePage(JNIEnv *env, jclass owner, jstring query) {
    (void)owner;
    if (query == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, query, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeBrowsePage((char *)text);
    (*env)->ReleaseStringUTFChars(env, query, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeBrowseFindPage(JNIEnv *env, jclass owner, jstring query) {
    (void)owner;
    if (query == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, query, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeBrowseFindPage((char *)text);
    (*env)->ReleaseStringUTFChars(env, query, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeBrowseDiagnostic(JNIEnv *env, jclass owner, jstring payload) {
    (void)owner;
    if (payload == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, payload, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeBrowseDiagnostic((char *)text);
    (*env)->ReleaseStringUTFChars(env, payload, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeAbout(JNIEnv *env, jclass owner,
    jstring origin, jstring dictionary) {
    (void)owner;
    if (origin == NULL || dictionary == NULL) return NULL;
    const char *originText = (*env)->GetStringUTFChars(env, origin, NULL);
    if (originText == NULL) return NULL;
    const char *dictText = (*env)->GetStringUTFChars(env, dictionary, NULL);
    if (dictText == NULL) {
        (*env)->ReleaseStringUTFChars(env, origin, originText);
        return NULL;
    }
    char *result = WudictGoBridgeAbout((char *)originText, (char *)dictText);
    (*env)->ReleaseStringUTFChars(env, dictionary, dictText);
    (*env)->ReleaseStringUTFChars(env, origin, originText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeOrphans(JNIEnv *env, jclass owner,
    jstring action, jstring body) {
    (void)owner;
    if (action == NULL || body == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *bodyText = (*env)->GetStringUTFChars(env, body, NULL);
    if (bodyText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeOrphans((char *)actionText, (char *)bodyText);
    (*env)->ReleaseStringUTFChars(env, body, bodyText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeRemoveLibrary(JNIEnv *env, jclass owner, jstring query) {
    (void)owner;
    if (query == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, query, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeRemoveLibrary((char *)text);
    (*env)->ReleaseStringUTFChars(env, query, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeLooks(JNIEnv *env, jclass owner, jstring argument) {
    (void)owner;
    if (argument == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, argument, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeLooks((char *)text);
    (*env)->ReleaseStringUTFChars(env, argument, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeGroups(JNIEnv *env, jclass owner,
    jstring action, jstring argument) {
    (void)owner;
    if (action == NULL || argument == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeGroups((char *)actionText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeLemmas(JNIEnv *env, jclass owner,
    jstring action, jstring code) {
    (void)owner;
    if (action == NULL || code == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *codeText = (*env)->GetStringUTFChars(env, code, NULL);
    if (codeText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeLemmas((char *)actionText, (char *)codeText);
    (*env)->ReleaseStringUTFChars(env, code, codeText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSetup(JNIEnv *env, jclass owner,
    jstring action, jstring argument) {
    (void)owner;
    if (action == NULL || argument == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeSetup((char *)actionText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeHeadwordExport(JNIEnv *env, jclass owner,
    jstring action, jstring argument) {
    (void)owner;
    if (action == NULL || argument == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeHeadwordExport((char *)actionText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeIntake(JNIEnv *env, jclass owner,
    jstring action, jstring argument) {
    (void)owner;
    if (action == NULL || argument == NULL) return NULL;
    const char *actionText = (*env)->GetStringUTFChars(env, action, NULL);
    if (actionText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, action, actionText);
        return NULL;
    }
    char *result = WudictGoBridgeIntake((char *)actionText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, action, actionText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jboolean JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativePower(JNIEnv *env, jclass owner,
    jstring state) {
    (void)owner;
    if (state == NULL) return JNI_FALSE;
    const char *text = (*env)->GetStringUTFChars(env, state, NULL);
    if (text == NULL) return JNI_FALSE;
    int ok = WudictGoBridgePower((char *)text);
    (*env)->ReleaseStringUTFChars(env, state, text);
    return ok ? JNI_TRUE : JNI_FALSE;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeReindex(JNIEnv *env, jclass owner, jstring action) {
    (void)owner;
    if (action == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, action, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeReindex((char *)text);
    (*env)->ReleaseStringUTFChars(env, action, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeIndexWork(JNIEnv *env, jclass owner,
    jstring action, jstring id, jstring body) {
    (void)owner;
    if (action == NULL || id == NULL || body == NULL) return NULL;
    const char *a = (*env)->GetStringUTFChars(env, action, NULL);
    if (a == NULL) return NULL;
    const char *i = (*env)->GetStringUTFChars(env, id, NULL);
    if (i == NULL) { (*env)->ReleaseStringUTFChars(env, action, a); return NULL; }
    const char *b = (*env)->GetStringUTFChars(env, body, NULL);
    if (b == NULL) {
        (*env)->ReleaseStringUTFChars(env, id, i);
        (*env)->ReleaseStringUTFChars(env, action, a);
        return NULL;
    }
    char *result = WudictGoBridgeIndexWork((char *)a, (char *)i, (char *)b);
    (*env)->ReleaseStringUTFChars(env, body, b);
    (*env)->ReleaseStringUTFChars(env, id, i);
    (*env)->ReleaseStringUTFChars(env, action, a);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

static JavaVM *busyVM;
static jclass busyClass;
static jmethodID busyMethod;

JNIEXPORT jboolean JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeInstallBusyObserver(JNIEnv *env, jclass owner) {
    if ((*env)->GetJavaVM(env, &busyVM) != JNI_OK) return JNI_FALSE;
    if (busyClass == NULL) {
        busyClass = (*env)->NewGlobalRef(env, owner);
        if (busyClass == NULL) return JNI_FALSE;
    }
    busyMethod = (*env)->GetStaticMethodID(env, busyClass, "onNativeBusy", "(Z)V");
    if (busyMethod == NULL) return JNI_FALSE;
    WudictGoBridgeInstallBusyObserver();
    return JNI_TRUE;
}

void wudictNotifyBusy(int busy) {
    if (busyVM == NULL || busyClass == NULL || busyMethod == NULL) return;
    JNIEnv *env = NULL;
    int attached = 0;
    jint state = (*busyVM)->GetEnv(busyVM, (void **)&env, JNI_VERSION_1_6);
    if (state == JNI_EDETACHED) {
        if ((*busyVM)->AttachCurrentThread(busyVM, &env, NULL) != JNI_OK) return;
        attached = 1;
    } else if (state != JNI_OK) {
        return;
    }
    (*env)->CallStaticVoidMethod(env, busyClass, busyMethod, busy ? JNI_TRUE : JNI_FALSE);
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionDescribe(env);
        (*env)->ExceptionClear(env);
    }
    if (attached) (*busyVM)->DetachCurrentThread(busyVM);
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeProtocol(JNIEnv *env, jclass owner) {
    (void)env;
    (void)owner;
    return (jint)WudictGoBridgeProtocol();
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeRuntimeStart(JNIEnv *env, jclass owner,
    jstring path, jstring flagsJSON, jstring version) {
    (void)owner;
    if (path == NULL || flagsJSON == NULL || version == NULL) return -1;
    const char *p = (*env)->GetStringUTFChars(env, path, NULL);
    if (p == NULL) return -1;
    const char *f = (*env)->GetStringUTFChars(env, flagsJSON, NULL);
    if (f == NULL) { (*env)->ReleaseStringUTFChars(env, path, p); return -1; }
    const char *v = (*env)->GetStringUTFChars(env, version, NULL);
    if (v == NULL) {
        (*env)->ReleaseStringUTFChars(env, flagsJSON, f);
        (*env)->ReleaseStringUTFChars(env, path, p);
        return -1;
    }
    jint result = (jint)WudictGoBridgeRuntimeStart((char *)p, (char *)f, (char *)v);
    (*env)->ReleaseStringUTFChars(env, version, v);
    (*env)->ReleaseStringUTFChars(env, flagsJSON, f);
    (*env)->ReleaseStringUTFChars(env, path, p);
    return result;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeRuntimeStop(JNIEnv *env, jclass owner) {
    (void)env;
    (void)owner;
    return (jint)WudictGoBridgeRuntimeStop();
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeConfigSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeConfigSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativePrefsSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgePrefsSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativePresetsSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgePresetsSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSavePreset(JNIEnv *env, jclass owner, jstring body) {
    (void)owner;
    if (body == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, body, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeSavePreset((char *)text);
    (*env)->ReleaseStringUTFChars(env, body, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeStyleSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeStyleSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSaveStyle(JNIEnv *env, jclass owner, jstring argument) {
    (void)owner;
    if (argument == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, argument, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeSaveStyle((char *)text);
    (*env)->ReleaseStringUTFChars(env, argument, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeUserFilesSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeUserFilesSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeUserFileOperation(JNIEnv *env, jclass owner,
    jstring method, jstring argument) {
    (void)owner;
    if (method == NULL || argument == NULL) return NULL;
    const char *methodText = (*env)->GetStringUTFChars(env, method, NULL);
    if (methodText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, method, methodText);
        return NULL;
    }
    char *result = WudictGoBridgeUserFileOperation((char *)methodText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, method, methodText);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeUserStorage(JNIEnv *env, jclass owner,
    jstring method, jstring argument) {
    (void)owner;
    if (method == NULL || argument == NULL) return NULL;
    const char *methodText = (*env)->GetStringUTFChars(env, method, NULL);
    if (methodText == NULL) return NULL;
    const char *argumentText = (*env)->GetStringUTFChars(env, argument, NULL);
    if (argumentText == NULL) {
        (*env)->ReleaseStringUTFChars(env, method, methodText);
        return NULL;
    }
    char *result = WudictGoBridgeUserStorage((char *)methodText, (char *)argumentText);
    (*env)->ReleaseStringUTFChars(env, argument, argumentText);
    (*env)->ReleaseStringUTFChars(env, method, methodText);
    if (result == NULL) return NULL;
    jstring text = (*env)->NewStringUTF(env, result);
    free(result);
    return text;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSavePrefs(JNIEnv *env, jclass owner, jstring body) {
    (void)owner;
    if (body == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, body, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeSavePrefs((char *)text);
    (*env)->ReleaseStringUTFChars(env, body, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSaveLanguage(JNIEnv *env, jclass owner, jstring body) {
    (void)owner;
    if (body == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, body, NULL);
    if (text == NULL) return NULL;
    char *result = WudictGoBridgeSaveLanguage((char *)text);
    (*env)->ReleaseStringUTFChars(env, body, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSystemLogSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeSystemLogSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeSystemLogOptions(JNIEnv *env, jclass owner,
    jboolean enabled, jboolean baseEvents, jboolean webEvents) {
    (void)env;
    (void)owner;
    return (jint)WudictGoBridgeSystemLogOptions(enabled, baseEvents, webEvents);
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeWebResource(JNIEnv *env, jclass owner,
    jstring rawURL, jstring headers) {
    (void)owner;
    if (rawURL == NULL || headers == NULL) return NULL;
    const char *text = (*env)->GetStringUTFChars(env, rawURL, NULL);
    if (text == NULL) return NULL;
    const char *headerText = (*env)->GetStringUTFChars(env, headers, NULL);
    if (headerText == NULL) {
        (*env)->ReleaseStringUTFChars(env, rawURL, text);
        return NULL;
    }
    char *result = WudictGoBridgeWebResource((char *)text, (char *)headerText);
    (*env)->ReleaseStringUTFChars(env, headers, headerText);
    (*env)->ReleaseStringUTFChars(env, rawURL, text);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeMediaOpen(JNIEnv *env, jclass owner,
    jstring rawURL, jstring headers) {
    (void)owner;
    if (rawURL == NULL || headers == NULL) return NULL;
    const char *url = (*env)->GetStringUTFChars(env, rawURL, NULL);
    if (url == NULL) return NULL;
    const char *head = (*env)->GetStringUTFChars(env, headers, NULL);
    if (head == NULL) {
        (*env)->ReleaseStringUTFChars(env, rawURL, url);
        return NULL;
    }
    char *result = WudictGoBridgeMediaOpen((char *)url, (char *)head);
    (*env)->ReleaseStringUTFChars(env, headers, head);
    (*env)->ReleaseStringUTFChars(env, rawURL, url);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jint JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeMediaRead(JNIEnv *env, jclass owner,
    jlong handle, jbyteArray dst, jint offset, jint length) {
    (void)owner;
    if (dst == NULL || offset < 0 || length < 0 ||
        offset > (*env)->GetArrayLength(env, dst) - length) return -2;
    if (length == 0) return 0;
    if (length > 65536) length = 65536;
    char *buffer = (char *)malloc((size_t)length);
    if (buffer == NULL) return -2;
    int n = WudictGoBridgeMediaRead((long long)handle, buffer, length);
    if (n > 0) (*env)->SetByteArrayRegion(env, dst, offset, n, (jbyte *)buffer);
    free(buffer);
    return n;
}

JNIEXPORT void JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeMediaClose(JNIEnv *env, jclass owner,
    jlong handle) {
    (void)env; (void)owner;
    WudictGoBridgeMediaClose((long long)handle);
}

JNIEXPORT jstring JNICALL
Java_com_dmshlabs_dictshelf_GoBridge_nativeConfigSummary(JNIEnv *env, jclass owner,
                                                             jstring path, jstring flagsJSON) {
    (void)owner;
    if (path == NULL || flagsJSON == NULL) return NULL;
    const char *pathChars = (*env)->GetStringUTFChars(env, path, NULL);
    if (pathChars == NULL) return NULL;
    const char *flagsChars = (*env)->GetStringUTFChars(env, flagsJSON, NULL);
    if (flagsChars == NULL) {
        (*env)->ReleaseStringUTFChars(env, path, pathChars);
        return NULL;
    }
    char *result = WudictGoBridgeConfigSummary((char *)pathChars, (char *)flagsChars);
    (*env)->ReleaseStringUTFChars(env, path, pathChars);
    (*env)->ReleaseStringUTFChars(env, flagsJSON, flagsChars);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}
