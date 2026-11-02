param([Parameter(Mandatory=$true)][string]$Executable)
$ErrorActionPreference = 'Stop'
$code = @'
using System;
using System.IO;
using System.Runtime.InteropServices;
public static class AppIcon {
 [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] static extern IntPtr BeginUpdateResource(string file, bool deleteExisting);
 [DllImport("kernel32.dll", EntryPoint="UpdateResourceW", SetLastError=true)] static extern bool UpdateResource(IntPtr h, IntPtr type, IntPtr name, ushort lang, byte[] data, uint size);
 [DllImport("kernel32.dll", SetLastError=true)] static extern bool EndUpdateResource(IntPtr h, bool discard);
 [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)] static extern IntPtr LoadLibraryEx(string file, IntPtr reserved, uint flags);
 [DllImport("kernel32.dll", EntryPoint="FindResourceW", SetLastError=true)] static extern IntPtr FindResource(IntPtr module, IntPtr name, IntPtr type);
 [DllImport("kernel32.dll")] static extern bool FreeLibrary(IntPtr module);
 public static void Write(string exe, byte[] ico) {
  if (ico.Length < 22 || BitConverter.ToUInt16(ico,0)!=0 || BitConverter.ToUInt16(ico,2)!=1) throw new Exception("Invalid ICO header");
  int n=BitConverter.ToUInt16(ico,4); if(n<1 || n>255 || 6+n*16>ico.Length) throw new Exception("Invalid ICO count");
  IntPtr h=BeginUpdateResource(exe,false); if(h==IntPtr.Zero) throw new System.ComponentModel.Win32Exception();
  bool ok=false;
  try {
   using(var ms=new MemoryStream()) using(var w=new BinaryWriter(ms)) {
    w.Write((ushort)0); w.Write((ushort)1); w.Write((ushort)n);
    for(int i=0;i<n;i++) {
     int p=6+i*16; int size=checked((int)BitConverter.ToUInt32(ico,p+8)); int off=checked((int)BitConverter.ToUInt32(ico,p+12));
     if(size<1 || off<0 || off>ico.Length-size) throw new Exception("Invalid ICO data range");
     byte[] data=new byte[size]; Buffer.BlockCopy(ico,off,data,0,size);
     if(!UpdateResource(h,(IntPtr)3,(IntPtr)(i+1),0,data,(uint)data.Length)) throw new System.ComponentModel.Win32Exception();
     w.Write(ico,p,12); w.Write((ushort)(i+1));
    }
    byte[] group=ms.ToArray();
    if(!UpdateResource(h,(IntPtr)14,(IntPtr)1,0,group,(uint)group.Length)) throw new System.ComponentModel.Win32Exception();
   }
   ok=true;
  } finally { if(!EndUpdateResource(h,!ok) && ok) throw new System.ComponentModel.Win32Exception(); }
  IntPtr module=LoadLibraryEx(exe,IntPtr.Zero,2); if(module==IntPtr.Zero) throw new System.ComponentModel.Win32Exception();
  try { if(FindResource(module,(IntPtr)1,(IntPtr)14)==IntPtr.Zero || FindResource(module,(IntPtr)1,(IntPtr)3)==IntPtr.Zero) throw new Exception("Embedded icon verification failed"); }
  finally { FreeLibrary(module); }
 }
}
'@
Add-Type -TypeDefinition $code
$icon = [Convert]::FromBase64String((Get-Content -Raw assets/icon.b64))
[AppIcon]::Write((Resolve-Path $Executable).Path, $icon)
Write-Host 'PASS embedded Windows icon resources'
