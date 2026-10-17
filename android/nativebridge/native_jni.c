// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cgo

#include <jni.h>
#include <stdlib.h>

extern int WudictGoBridgeProtocol(void);
extern char *WudictGoBridgeConfigSummary(char *path, char *flagsJSON);
extern void WudictGoBridgeInstallBusyObserver(void);
extern int WudictGoBridgeRuntimeStart(char *path, char *flagsJSON, char *token, char *version);
extern int WudictGoBridgeRuntimeStop(void);
extern char *WudictGoBridgeConfigSnapshot(void);
extern char *WudictGoBridgePrefsSnapshot(void);
extern char *WudictGoBridgeBrowsePage(char *query);
extern char *WudictGoBridgeBrowseDiagnostic(char *payload);
extern char *WudictGoBridgeAbout(char *origin, char *dictionary);
extern char *WudictGoBridgeOrphans(char *action, char *body);
extern char *WudictGoBridgeRemoveLibrary(char *query);
extern char *WudictGoBridgeLooks(char *argument);
extern char *WudictGoBridgeGroups(char *action, char *argument);
extern char *WudictGoBridgeLemmas(char *action, char *code);
extern char *WudictGoBridgeSetup(char *action, char *argument);
extern char *WudictGoBridgeIntake(char *action, char *argument);
extern char *WudictGoBridgeReindex(char *action);
extern char *WudictGoBridgeIndexWork(char *action, char *id, char *body);
extern char *WudictGoBridgePresetsSnapshot(void);
extern char *WudictGoBridgeSavePreset(char *body);
extern char *WudictGoBridgeStyleSnapshot(void);
extern char *WudictGoBridgeSaveStyle(char *argument);
extern char *WudictGoBridgeUserFilesSnapshot(void);
extern char *WudictGoBridgeUserFileOperation(char *method, char *argument);
extern char *WudictGoBridgeSavePrefs(char *body);
extern int WudictGoBridgeDictStreamStart(void);
extern int WudictGoBridgeRescanStreamStart(void);
extern int WudictGoBridgeRescanIndexesStart(char *origin, char *request);
extern int WudictGoBridgeIngestStart(char *origin, char *query);
extern int WudictGoBridgeSearchStreamStart(char *origin, char *request);
extern int WudictGoBridgeBrowseStreamStart(char *method, char *query);
extern char *WudictGoBridgeDictStreamPoll(int id);
extern void WudictGoBridgeDictStreamCancel(int id);

JNIEXPORT jint JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeDictStreamStart(JNIEnv *env, jclass owner) {
    (void)env; (void)owner;
    return (jint)WudictGoBridgeDictStreamStart();
}

JNIEXPORT jint JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeRescanStreamStart(JNIEnv *env, jclass owner) {
    (void)env; (void)owner;
    return (jint)WudictGoBridgeRescanStreamStart();
}

JNIEXPORT jint JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeRescanIndexesStart(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeIngestStart(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeSearchStreamStart(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeBrowseStreamStart(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeDictStreamPoll(JNIEnv *env, jclass owner, jint id) {
    (void)owner;
    char *result = WudictGoBridgeDictStreamPoll((int)id);
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT void JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeDictStreamCancel(JNIEnv *env, jclass owner, jint id) {
    (void)env; (void)owner;
    WudictGoBridgeDictStreamCancel((int)id);
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeBrowsePage(JNIEnv *env, jclass owner, jstring query) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeBrowseDiagnostic(JNIEnv *env, jclass owner, jstring payload) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeAbout(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeOrphans(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeRemoveLibrary(JNIEnv *env, jclass owner, jstring query) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeLooks(JNIEnv *env, jclass owner, jstring argument) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeGroups(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeLemmas(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeSetup(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeIntake(JNIEnv *env, jclass owner,
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

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeReindex(JNIEnv *env, jclass owner, jstring action) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeIndexWork(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeInstallBusyObserver(JNIEnv *env, jclass owner) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeProtocol(JNIEnv *env, jclass owner) {
    (void)env;
    (void)owner;
    return (jint)WudictGoBridgeProtocol();
}

JNIEXPORT jint JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeRuntimeStart(JNIEnv *env, jclass owner,
    jstring path, jstring flagsJSON, jstring token, jstring version) {
    (void)owner;
    if (path == NULL || flagsJSON == NULL || token == NULL || version == NULL) return -1;
    const char *p = (*env)->GetStringUTFChars(env, path, NULL);
    if (p == NULL) return -1;
    const char *f = (*env)->GetStringUTFChars(env, flagsJSON, NULL);
    if (f == NULL) { (*env)->ReleaseStringUTFChars(env, path, p); return -1; }
    const char *t = (*env)->GetStringUTFChars(env, token, NULL);
    if (t == NULL) {
        (*env)->ReleaseStringUTFChars(env, flagsJSON, f);
        (*env)->ReleaseStringUTFChars(env, path, p);
        return -1;
    }
    const char *v = (*env)->GetStringUTFChars(env, version, NULL);
    if (v == NULL) {
        (*env)->ReleaseStringUTFChars(env, token, t);
        (*env)->ReleaseStringUTFChars(env, flagsJSON, f);
        (*env)->ReleaseStringUTFChars(env, path, p);
        return -1;
    }
    jint result = (jint)WudictGoBridgeRuntimeStart((char *)p, (char *)f, (char *)t, (char *)v);
    (*env)->ReleaseStringUTFChars(env, version, v);
    (*env)->ReleaseStringUTFChars(env, token, t);
    (*env)->ReleaseStringUTFChars(env, flagsJSON, f);
    (*env)->ReleaseStringUTFChars(env, path, p);
    return result;
}

JNIEXPORT jint JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeRuntimeStop(JNIEnv *env, jclass owner) {
    (void)env;
    (void)owner;
    return (jint)WudictGoBridgeRuntimeStop();
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeConfigSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeConfigSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativePrefsSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgePrefsSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativePresetsSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgePresetsSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeSavePreset(JNIEnv *env, jclass owner, jstring body) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeStyleSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeStyleSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeSaveStyle(JNIEnv *env, jclass owner, jstring argument) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeUserFilesSnapshot(JNIEnv *env, jclass owner) {
    (void)owner;
    char *result = WudictGoBridgeUserFilesSnapshot();
    if (result == NULL) return NULL;
    jstring answer = (*env)->NewStringUTF(env, result);
    free(result);
    return answer;
}

JNIEXPORT jstring JNICALL
Java_com_legbehindneck_wudict_GoBridge_nativeUserFileOperation(JNIEnv *env, jclass owner,
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
Java_com_legbehindneck_wudict_GoBridge_nativeSavePrefs(JNIEnv *env, jclass owner, jstring body) {
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
Java_com_legbehindneck_wudict_GoBridge_nativeConfigSummary(JNIEnv *env, jclass owner,
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
