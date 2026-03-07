using System;
using System.IO;
using System.Text;
using System.Collections.Generic;
using System.Linq;

namespace SiigoExplorer
{
    class Program
    {
        static Encoding enc = Encoding.GetEncoding(1252);

        static void Main(string[] args)
        {
            Console.OutputEncoding = enc;
            Console.WriteLine("=== SIIGO ISAM Data Reader v2 ===\n");

            string dataPath = @"C:\DEMOS01\";

            // Get record size from ISAM header and read records
            Console.WriteLine("========== TERCEROS / CLIENTES (Z17) ==========");
            var z17Records = ReadIsamRecords(Path.Combine(dataPath, "Z17"));
            foreach (var rec in z17Records.Take(20))
            {
                ParseTercero(rec);
            }
            Console.WriteLine($"  Total registros: {z17Records.Count}");

            Console.WriteLine("\n========== PRODUCTOS (Z06) ==========");
            var z06Records = ReadIsamRecords(Path.Combine(dataPath, "Z06"));
            foreach (var rec in z06Records.Take(20))
            {
                ParseProducto(rec);
            }
            Console.WriteLine($"  Total registros: {z06Records.Count}");

            Console.WriteLine("\n========== PLAN DE CUENTAS (C03) ==========");
            var c03Records = ReadIsamRecords(Path.Combine(dataPath, "C03"));
            foreach (var rec in c03Records.Take(20))
            {
                string text = enc.GetString(rec).Replace('\0', ' ').Trim();
                if (text.Length > 10)
                    Console.WriteLine($"  {text.Substring(0, Math.Min(120, text.Length))}");
            }
            Console.WriteLine($"  Total registros: {c03Records.Count}");

            Console.WriteLine("\n========== MOVIMIENTOS (Z49) ==========");
            var z49Records = ReadIsamRecords(Path.Combine(dataPath, "Z49"));
            foreach (var rec in z49Records.Take(10))
            {
                string text = enc.GetString(rec).Replace('\0', '|').Trim();
                Console.WriteLine($"  [{text.Substring(0, Math.Min(150, text.Length))}]");
            }
            Console.WriteLine($"  Total registros: {z49Records.Count}");

            Console.WriteLine("\n========== Z70 (Comprobantes) ==========");
            var z70Records = ReadIsamRecords(Path.Combine(dataPath, "Z70"));
            foreach (var rec in z70Records.Take(10))
            {
                string text = enc.GetString(rec).Replace('\0', '|').Trim();
                Console.WriteLine($"  [{text.Substring(0, Math.Min(150, text.Length))}]");
            }
            Console.WriteLine($"  Total registros: {z70Records.Count}");

            Console.WriteLine("\n========== Z90ES (Módulos/Permisos) ==========");
            var z90Records = ReadIsamRecords(Path.Combine(dataPath, "Z90ES"));
            foreach (var rec in z90Records.Take(15))
            {
                string text = enc.GetString(rec).Replace('\0', '|').Trim();
                Console.WriteLine($"  [{text.Substring(0, Math.Min(120, text.Length))}]");
            }
            Console.WriteLine($"  Total registros: {z90Records.Count}");

            Console.WriteLine("\n========== Z91PRO (Productos maestro) ==========");
            var z91Records = ReadIsamRecords(Path.Combine(dataPath, "Z91PRO"));
            foreach (var rec in z91Records.Take(15))
            {
                ParseProductoZ91(rec);
            }
            Console.WriteLine($"  Total registros: {z91Records.Count}");

            Console.WriteLine("\n========== TOP 30 ARCHIVOS ==========");
            var files = Directory.GetFiles(dataPath)
                .Where(f => !f.EndsWith(".idx", StringComparison.OrdinalIgnoreCase)
                         && !f.EndsWith(".mdb", StringComparison.OrdinalIgnoreCase)
                         && !f.EndsWith(".rar", StringComparison.OrdinalIgnoreCase)
                         && !f.EndsWith(".gnt", StringComparison.OrdinalIgnoreCase)
                         && !f.EndsWith(".REG", StringComparison.OrdinalIgnoreCase)
                         && !f.EndsWith(".TMP", StringComparison.OrdinalIgnoreCase))
                .Select(f => new FileInfo(f))
                .Where(f => f.Length > 4096) // Only meaningful files
                .OrderByDescending(f => f.Length)
                .Take(30);

            foreach (var fi in files)
            {
                int recSize = GetRecordSize(fi.FullName);
                int recCount = ReadIsamRecords(fi.FullName).Count;
                Console.WriteLine($"  {fi.Name,-20} {fi.Length,12:N0} bytes  recSize={recSize,5}  records={recCount}");
            }

            Console.WriteLine("\n=== Fin ===");
        }

        static int GetRecordSize(string filePath)
        {
            try
            {
                byte[] header = new byte[128];
                using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
                {
                    fs.Read(header, 0, 128);
                }
                // Record size is stored at offset 0x38 as big-endian 16-bit or 32-bit
                // Actually it's at different positions depending on format
                // For IDXFORMAT=8, let's check common locations

                // Check offset 0x38 (big-endian 16-bit)
                int size1 = (header[0x38] << 8) | header[0x39];
                if (size1 > 0 && size1 < 65000) return size1;

                // Check offset 0x34
                int size2 = (header[0x34] << 8) | header[0x35];
                if (size2 > 0 && size2 < 65000) return size2;

                return 0;
            }
            catch { return 0; }
        }

        static List<byte[]> ReadIsamRecords(string filePath)
        {
            var records = new List<byte[]>();
            if (!File.Exists(filePath)) return records;

            try
            {
                byte[] data;
                using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
                {
                    data = new byte[fs.Length];
                    fs.Read(data, 0, data.Length);
                }

                int recSize = GetRecordSize(filePath);
                if (recSize <= 0 || recSize > 60000) return records;

                // Scan for records: they start with the record size marker (2 bytes matching recSize)
                // followed by data. The marker format varies but commonly:
                // byte[0] = flags/status, byte[1] = recSize high byte, or full 2-byte marker

                // In IDXFORMAT=8, data records have a 2-byte prefix:
                // First byte: status (0x45 = active record, etc.)
                // Second byte: part of size or flags
                // The actual marker seems to be the recSize as big-endian 16-bit

                byte recHi = (byte)((recSize >> 8) & 0xFF);
                byte recLo = (byte)(recSize & 0xFF);

                for (int pos = 0x800; pos < data.Length - recSize; pos++)
                {
                    // Look for record markers
                    // Pattern: status_byte + recSize(2 bytes big-endian) matches
                    // We saw: 459E for recSize=059E, meaning byte pattern is [status][recHi][recLo]
                    // But 45 9E: 45=status(E=active?), 9E = recLo. Hmm.

                    // Actually looking at it: the 2 bytes at record start encode something like
                    // [flags | recHi] [recLo] where the low bits of recHi are the record size upper bits

                    // Simpler: scan for the 2nd byte = recLo and check if the area after has readable data
                    if (data[pos + 1] == recLo && (data[pos] & 0x0F) == recHi)
                    {
                        // Check if this looks like a valid record (has some readable text)
                        int textStart = pos + 2;
                        int readableCount = 0;
                        for (int i = textStart; i < textStart + 30 && i < data.Length; i++)
                        {
                            if (data[i] >= 0x20 && data[i] < 0xFF) readableCount++;
                        }

                        if (readableCount > 15) // At least half should be readable
                        {
                            byte[] record = new byte[recSize];
                            Array.Copy(data, textStart, record, 0, Math.Min(recSize, data.Length - textStart));
                            records.Add(record);
                            pos += recSize; // Skip to next potential record
                        }
                    }
                }
            }
            catch { }

            return records;
        }

        static void ParseTercero(byte[] rec)
        {
            // Z17 record structure (1438 bytes = 0x59E):
            // Based on what we see: key(18) + type(2) + nit(10) + padding + date(8) + name(40) + ...
            string raw = enc.GetString(rec);

            string key = SafeSub(raw, 0, 18).Trim();       // G001000000000020XX or similar
            string tipo = SafeSub(raw, 18, 4).Trim();       // Type code
            string nit = SafeSub(raw, 22, 10).Trim();       // NIT/CC number
            string fecha = SafeSub(raw, 32, 8).Trim();      // Date YYYYMMDD
            string nombre = SafeSub(raw, 40, 50).Trim();    // Name

            if (string.IsNullOrWhiteSpace(nombre) && raw.Length > 20)
            {
                // Try alternative layout - search for the readable text
                int nameStart = -1;
                for (int i = 10; i < Math.Min(100, raw.Length); i++)
                {
                    if (raw[i] >= 'A' && raw[i] <= 'Z' && i + 3 < raw.Length
                        && raw[i+1] >= 'A' && raw[i+1] <= 'z')
                    {
                        nameStart = i;
                        break;
                    }
                }
                if (nameStart >= 0)
                {
                    nombre = SafeSub(raw, nameStart, 50).Trim();
                }
            }

            if (!string.IsNullOrWhiteSpace(nombre))
            {
                Console.WriteLine($"  Tercero: [{key}] {nombre}");
            }
        }

        static void ParseProducto(byte[] rec)
        {
            string raw = enc.GetString(rec);
            // Search for readable product name
            int nameStart = -1;
            for (int i = 5; i < Math.Min(200, raw.Length); i++)
            {
                if (raw[i] >= 'A' && raw[i] <= 'Z' && i + 3 < raw.Length
                    && raw[i+1] >= ' ')
                {
                    nameStart = i;
                    break;
                }
            }

            string key = SafeSub(raw, 0, 20).Trim();
            string nombre = nameStart >= 0 ? SafeSub(raw, nameStart, 60).Trim() : "";

            if (!string.IsNullOrWhiteSpace(nombre) && nombre.Length > 2)
            {
                Console.WriteLine($"  Producto: [{SafeSub(key, 0, 20)}] {nombre}");
            }
        }

        static void ParseProductoZ91(byte[] rec)
        {
            string raw = enc.GetString(rec);
            string firstChunk = SafeSub(raw, 0, 120).Replace('\0', '|').Trim();
            if (firstChunk.Length > 5)
                Console.WriteLine($"  Z91: {firstChunk}");
        }

        static string SafeSub(string s, int start, int len)
        {
            if (s == null || start >= s.Length) return "";
            if (start + len > s.Length) len = s.Length - start;
            return s.Substring(start, len);
        }
    }
}
