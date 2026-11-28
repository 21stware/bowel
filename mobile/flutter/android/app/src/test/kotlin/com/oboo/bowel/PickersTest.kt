package com.oboo.bowel

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PickersTest {
    @Test
    fun androidTvStubIsNoPicker() {
        // What ACTION_OPEN_DOCUMENT resolves to on the Android TV emulator.
        assertFalse(Pickers.real(listOf("com.android.tv.frameworkpackagestubs")))
        assertFalse(Pickers.real(listOf("com.google.android.tv.frameworkpackagestubs")))
        assertFalse(Pickers.real(emptyList()))
    }

    @Test
    fun aRealPickerBesideTheStubCounts() {
        assertTrue(Pickers.real(listOf("com.google.android.documentsui")))
        assertTrue(Pickers.real(listOf("com.android.tv.frameworkpackagestubs", "com.example.files")))
        assertTrue(Pickers.real(listOf("com.google.android.providers.media.module")))
    }
}
