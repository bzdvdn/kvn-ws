# Keep Kotlin serialization
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt

-keepclassmembers class kotlinx.serialization.json.** {
    *** Companion;
}
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}

-keep,includedescriptorclasses class com.kvn.client.**$$serializer { *; }
-keepclassmembers class com.kvn.client.** {
    *** Companion;
}
-keepclasseswithmembers class com.kvn.client.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Keep ML Kit
-keep class com.google.mlkit.** { *; }
-dontwarn com.google.mlkit.**

# Keep CameraX
-keep class androidx.camera.** { *; }

# Keep ZXing
-keep class com.google.zxing.** { *; }
