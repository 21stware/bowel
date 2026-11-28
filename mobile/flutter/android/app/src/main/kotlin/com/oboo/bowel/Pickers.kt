package com.oboo.bowel

/**
 * Which apps that answer a picker's intent can really pick.
 *
 * Android TV answers the intents its builds ship no app for with stubs
 * (`com.android.tv.frameworkpackagestubs`, `Stubs$DocumentsStub` for
 * ACTION_OPEN_DOCUMENT), which open and close at once and return nothing. A
 * picker that only resolves to one of those is no picker.
 */
internal object Pickers {
    /** Whether any of the packages that answer is not a framework stub. */
    fun real(packages: List<String>): Boolean = packages.any { !isStub(it) }

    fun isStub(pkg: String): Boolean = pkg.endsWith(".frameworkpackagestubs")
}
