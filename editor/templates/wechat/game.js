import 'runtime/wasm_exec';

const go = new Go();
const fileSystem = wx.getFileSystemManager();
const textEncoder = require('./runtime/text_encoder');
const decodeCatalog = (data) => {
  let offset = 0;
  const take = (length) => {
    if (offset + length > data.length) throw new Error("truncated resource catalog");
    const value = data.subarray(offset, offset + length);
    offset += length;
    return value;
  };
  const uvarint = () => {
    let value = 0;
    let factor = 1;
    for (let index = 0; index < 10; index++) {
      const byte = take(1)[0];
      value += (byte & 0x7f) * factor;
      if ((byte & 0x80) === 0) {
        if (!Number.isSafeInteger(value)) throw new Error("resource catalog integer overflow");
        return value;
      }
      factor *= 128;
    }
    throw new Error("invalid resource catalog varint");
  };
  const decoder = new textEncoder.TextDecoder();
  const string = () => decoder.decode(take(uvarint()));
  const strings = () => Array.from({length: uvarint()}, string);
  const magic = [79, 78, 73, 67, 65, 84, 50, 0];
  if (!magic.every((value) => take(1)[0] === value)) throw new Error("invalid resource catalog marker");
  if (uvarint() !== 2) throw new Error("unsupported resource catalog version");
  const packages = Array.from({length: uvarint()}, () => {
    const name = string();
    const file = string();
    take(32);
    uvarint();
    const requiredByte = take(1)[0];
    if (requiredByte > 1) throw new Error("invalid resource catalog required flag");
    const required = requiredByte === 1;
    const dependencies = strings();
    const assets = uvarint();
    for (let index = 0; index < assets; index++) {
      string();
      uvarint();
      take(4);
      take(16);
    }
    return {name, file, required, dependencies};
  });
  if (offset !== data.length) throw new Error("resource catalog has trailing data");
  return packages;
};
const catalogData = new Uint8Array(fileSystem.readFileSync("resources/catalog.bin"));
const catalog = decodeCatalog(catalogData);
const packages = new Map(catalog.map((item) => [item.name, item]));
const required = new Set();
const add = (name) => {
  if (required.has(name)) return;
  const item = packages.get(name);
  if (!item) throw new Error(`resource package not found: ${name}`);
  item.dependencies?.forEach(add);
  required.add(name);
};
catalog.filter((item) => item.required).forEach((item) => add(item.name));
globalThis.__onixCatalogData = catalogData;
globalThis.__onixPackageData = {};
required.forEach((name) => {
  const item = packages.get(name);
  globalThis.__onixPackageData[item.file] = new Uint8Array(fileSystem.readFileSync(`resources/${item.file}`));
});
WXWebAssembly.instantiate("resources/__WASM_FILENAME__", go.importObject).then(result => {
  go.run(result.instance);
}).catch(err => {
  console.log(err)
});
