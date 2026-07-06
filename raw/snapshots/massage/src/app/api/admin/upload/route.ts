import { promises as fs } from 'node:fs';
import path from 'node:path';
import { validateUploadFiles } from '@/app/api/admin/upload/upload-validation';
import { requireRole } from '@/lib/auth/guards';
import { errorResponse } from '@/lib/auth/http';
import { isSupabaseStorageConfigured, uploadShopImageToSupabase } from '@/lib/server/supabase-storage';

export async function POST(request: Request) {
  try {
    // Only ADMIN or OWNER can upload images
    await requireRole('ADMIN', 'OWNER');

    const formData = await request.formData();
    const files = formData.getAll('file').filter((entry): entry is File => entry instanceof File);
    const validation = await validateUploadFiles(files);

    if (!validation.ok) {
      return Response.json({ error: validation.error }, { status: validation.status });
    }

    const uploadUrls: string[] = [];
    // Persist to Supabase Storage in production (the serverless filesystem is ephemeral).
    // Fall back to the local public directory only when storage is not configured (local dev).
    const useSupabaseStorage = isSupabaseStorageConfigured();
    const publicUploadDir = path.join(process.cwd(), 'public', 'uploads');

    if (!useSupabaseStorage) {
      await fs.mkdir(publicUploadDir, { recursive: true });
    }

    for (const file of files) {
      const bytes = await file.arrayBuffer();
      const buffer = Buffer.from(bytes);

      // Unique file name to prevent duplication/overwriting
      const uniqueSuffix = `${Date.now()}-${Math.round(Math.random() * 1e9)}`;
      const ext = path.extname(file.name).toLowerCase();
      const filename = `${uniqueSuffix}${ext}`;

      if (useSupabaseStorage) {
        const publicUrl = await uploadShopImageToSupabase(
          `shops/${filename}`,
          buffer,
          file.type || 'application/octet-stream',
        );

        if (!publicUrl) {
          throw new Error('이미지 저장소 연결에 실패했습니다. 잠시 후 다시 시도해 주세요.');
        }

        uploadUrls.push(publicUrl);
      } else {
        const filepath = path.join(publicUploadDir, filename);
        await fs.writeFile(filepath, buffer);
        uploadUrls.push(`/uploads/${filename}`);
      }
    }

    return Response.json({ urls: uploadUrls }, { status: 201 });
  } catch (error) {
    return errorResponse(error);
  }
}
